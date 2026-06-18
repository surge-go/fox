package fox

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	coreerrors "github.com/surge-go/fox/core/errors"
)

func boolPtr(v bool) *bool {
	return &v
}

func expectPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

type customErrors struct {
	Err
}

func (customErrors) ErrServer() *coreerrors.Error {
	return coreerrors.NewWithStatus(20001, http.StatusInternalServerError, "custom internal error")
}

func (customErrors) ErrBadRequest() *coreerrors.Error {
	return coreerrors.NewWithStatus(10001, http.StatusBadRequest, "custom bad request")
}

func (customErrors) ErrInvalidParams() *coreerrors.Error {
	return coreerrors.NewWithStatus(10003, http.StatusBadRequest, "custom invalid params")
}

func (customErrors) ErrUnauthorized() *coreerrors.Error {
	return coreerrors.NewWithStatus(10002, http.StatusUnauthorized, "custom unauthorized")
}

func TestNewAppliesDefaultConfig(t *testing.T) {
	e := New(nil)
	if e == nil {
		t.Fatal("New(nil) returned nil")
	}
	if e.cfg == nil || e.cfg.Addr != defaultAddr {
		t.Fatalf("addr = %q, want %q", e.cfg.Addr, defaultAddr)
	}
	if e.mode != ModeRelease {
		t.Fatalf("mode = %q, want %q", e.mode, ModeRelease)
	}
	if e.cfg.ShutdownTimeout != defaultShutdownTimeout {
		t.Fatalf("shutdown timeout = %s, want %s", e.cfg.ShutdownTimeout, defaultShutdownTimeout)
	}
}

func TestNewKeepsCustomShutdownTimeout(t *testing.T) {
	timeout := 5 * time.Second
	e := New(&Config{Addr: ":0", ShutdownTimeout: timeout})
	if e.cfg.ShutdownTimeout != timeout {
		t.Fatalf("shutdown timeout = %s, want %s", e.cfg.ShutdownTimeout, timeout)
	}
}

func TestNewRejectsNegativeShutdownTimeout(t *testing.T) {
	expectPanic(t, func() {
		New(&Config{Addr: ":0", ShutdownTimeout: -time.Second})
	})
}

func TestNewRejectsInvalidAddrPort(t *testing.T) {
	tests := []string{
		"localhost:http",
		"127.0.0.1:65536",
		":65536",
	}

	for _, addr := range tests {
		t.Run(addr, func(t *testing.T) {
			expectPanic(t, func() {
				New(&Config{Addr: addr})
			})
		})
	}
}

func TestEngineUsesCustomErrorFactoryForBindFailure(t *testing.T) {
	e := New(&Config{Addr: ":0", Mode: ModeTest, PrintRoutes: boolPtr(false)}, &customErrors{})
	e.POST("/users", func(c *Context) {
		var req struct {
			Name string `json:"name"`
		}
		if err := c.BindJSON(&req); err != nil {
			return
		}
		c.Ok(req)
	})

	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"code":10003`) ||
		!strings.Contains(body, `"message":"custom invalid params"`) {
		t.Fatalf("body = %q, want custom invalid params response", body)
	}
}

func TestEngineErrorFactoryIsIsolatedPerEngine(t *testing.T) {
	customEngine := New(&Config{Addr: ":0", Mode: ModeTest, PrintRoutes: boolPtr(false)}, &customErrors{})
	defaultEngine := New(&Config{Addr: ":0", Mode: ModeTest, PrintRoutes: boolPtr(false)})

	registerBindRoute := func(e *Engine) {
		e.POST("/users", func(c *Context) {
			var req struct {
				Name string `json:"name"`
			}
			if err := c.BindJSON(&req); err != nil {
				return
			}
			c.Ok(req)
		})
	}
	registerBindRoute(customEngine)
	registerBindRoute(defaultEngine)

	newBadJSONRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader("{"))
		req.Header.Set("Content-Type", "application/json")
		return req
	}

	customRec := httptest.NewRecorder()
	customEngine.ServeHTTP(customRec, newBadJSONRequest())
	if body := customRec.Body.String(); !strings.Contains(body, `"code":10003`) {
		t.Fatalf("custom engine body = %q, want custom code", body)
	}

	defaultRec := httptest.NewRecorder()
	defaultEngine.ServeHTTP(defaultRec, newBadJSONRequest())
	if body := defaultRec.Body.String(); strings.Contains(body, `"code":10003`) ||
		!strings.Contains(body, `"message":"invalid params"`) {
		t.Fatalf("default engine body = %q, want default invalid params", body)
	}
}

func TestEngineUsesCustomErrorFactoryForPanicRecovery(t *testing.T) {
	e := New(&Config{Addr: ":0", Mode: ModeTest, PrintRoutes: boolPtr(false)}, &customErrors{})
	e.GET("/panic", func(c *Context) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"code":20001`) ||
		!strings.Contains(body, `"message":"custom internal error"`) {
		t.Fatalf("body = %q, want custom internal error response", body)
	}
}

func TestMiddlewareCanUseContextErrors(t *testing.T) {
	e := New(&Config{Addr: ":0", Mode: ModeTest, PrintRoutes: boolPtr(false)}, &customErrors{})
	e.Use(func(c *Context) {
		c.Fail(c.Errors().ErrUnauthorized())
	})
	e.GET("/private", func(c *Context) {
		c.Ok("private")
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"code":10002`) ||
		!strings.Contains(body, `"message":"custom unauthorized"`) {
		t.Fatalf("body = %q, want custom unauthorized response", body)
	}
}

func TestFailNilWritesServerError(t *testing.T) {
	e := New(&Config{Addr: ":0", Mode: ModeTest, PrintRoutes: boolPtr(false)}, &customErrors{})
	e.GET("/fail-nil", func(c *Context) {
		c.Fail(nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/fail-nil", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"code":20001`) ||
		!strings.Contains(body, `"message":"custom internal error"`) {
		t.Fatalf("body = %q, want custom internal error response", body)
	}
}

func TestToGinHandlerDefaultsNilErrorFactory(t *testing.T) {
	called := false
	handler := toGinHandler(func(c *Context) {
		called = true
		if c.errors == nil {
			t.Fatal("context errors is nil")
		}
		if _, ok := c.errors.(*Err); !ok {
			t.Fatalf("context errors = %T, want *Err", c.errors)
		}
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	gc, _ := gin.CreateTestContext(rec)
	gc.Request = req

	handler(gc)

	if !called {
		t.Fatal("handler was not called")
	}
}

func TestEngineRegistersRouteAndRecordsSnapshot(t *testing.T) {
	e := New(&Config{Addr: ":0", Mode: ModeRelease, PrintRoutes: boolPtr(false)})

	e.GET("/ping", func(c *Context) {
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "pong" {
		t.Fatalf("body = %q, want pong", body)
	}

	routes := e.routeSnapshot()
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}
	if routes[0].Method != http.MethodGet || routes[0].Path != "/ping" {
		t.Fatalf("route = %s %s, want GET /ping", routes[0].Method, routes[0].Path)
	}
	if !strings.Contains(routes[0].Handler, "func") {
		t.Fatalf("handler = %q, want anonymous function name", routes[0].Handler)
	}
}

func TestRouteGroupCombinesPrefixAndMiddleware(t *testing.T) {
	e := New(&Config{Addr: ":0", Mode: ModeTest, PrintRoutes: boolPtr(false)})

	api := e.Group("/api", func(c *Context) {
		c.SetHeader("X-Group", "yes")
		c.Next()
	})
	api.Group("/v1").GET("/users", func(c *Context) {
		c.String(http.StatusAccepted, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if got := rec.Header().Get("X-Group"); got != "yes" {
		t.Fatalf("X-Group = %q, want yes", got)
	}
}

func TestRouteGroupJoinsPathWithoutLeadingSlash(t *testing.T) {
	e := New(&Config{Addr: ":0", Mode: ModeTest, PrintRoutes: boolPtr(false)})

	e.Group("/api").GET("users", func(c *Context) {
		c.String(http.StatusOK, "users")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "users" {
		t.Fatalf("body = %q, want users", body)
	}

	routes := e.routeSnapshot()
	if len(routes) != 1 || routes[0].Path != "/api/users" {
		t.Fatalf("routes = %+v, want one route at /api/users", routes)
	}
}

func TestUseRejectsNilMiddleware(t *testing.T) {
	e := New(&Config{Addr: ":0", Mode: ModeTest, PrintRoutes: boolPtr(false)})

	expectPanic(t, func() {
		e.Use(nil)
	})
}

func TestRouteGroupRejectsNilMiddleware(t *testing.T) {
	e := New(&Config{Addr: ":0", Mode: ModeTest, PrintRoutes: boolPtr(false)})

	t.Run("engine group", func(t *testing.T) {
		expectPanic(t, func() {
			e.Group("/api", nil)
		})
	})

	t.Run("route group use", func(t *testing.T) {
		group := e.Group("/api")
		expectPanic(t, func() {
			group.Use(nil)
		})
	})

	t.Run("child group", func(t *testing.T) {
		group := e.Group("/api")
		expectPanic(t, func() {
			group.Group("/v1", nil)
		})
	})
}

func TestPrintRoutesTakesPrecedenceOverEnableLogger(t *testing.T) {
	enableLogger := true
	printRoutes := false
	e := New(&Config{
		Addr:         ":0",
		Mode:         ModeTest,
		EnableLogger: &enableLogger,
		PrintRoutes:  &printRoutes,
	})
	if e.cfg.printRoutesEnabled() {
		t.Fatal("printRoutesEnabled() = true, want false")
	}
}

func TestNewRegistersDefaultRecovery(t *testing.T) {
	e := New(&Config{Addr: ":0", Mode: ModeTest, PrintRoutes: boolPtr(false)})
	e.GET("/panic", func(c *Context) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestOkAbortsRemainingHandlers(t *testing.T) {
	e := New(&Config{Addr: ":0", Mode: ModeTest, PrintRoutes: boolPtr(false)})
	called := false
	e.GET("/ok", func(c *Context) {
		c.Ok("done")
	}, func(c *Context) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if called {
		t.Fatal("expected second handler to be skipped")
	}
}

func TestStaticRejectsUninitializedEngine(t *testing.T) {
	var e Engine
	expectPanic(t, func() {
		e.Static("/assets", ".")
	})
}

func TestServeHTTPDoesNotSealRoutes(t *testing.T) {
	e := New(&Config{Addr: ":0", Mode: ModeTest, PrintRoutes: boolPtr(false)})
	e.GET("/first", func(c *Context) {
		c.String(http.StatusOK, "first")
	})

	firstReq := httptest.NewRequest(http.MethodGet, "/first", nil)
	firstRec := httptest.NewRecorder()
	e.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d", firstRec.Code, http.StatusOK)
	}

	e.GET("/second", func(c *Context) {
		c.String(http.StatusOK, "second")
	})

	secondReq := httptest.NewRequest(http.MethodGet, "/second", nil)
	secondRec := httptest.NewRecorder()
	e.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("second status = %d, want %d", secondRec.Code, http.StatusOK)
	}
}

func TestSealedRoutesRejectRegistration(t *testing.T) {
	newSealedEngine := func() *Engine {
		e := New(&Config{Addr: ":0", Mode: ModeTest, PrintRoutes: boolPtr(false)})
		e.sealRoutes()
		return e
	}

	t.Run("handle", func(t *testing.T) {
		e := newSealedEngine()
		expectPanic(t, func() {
			e.GET("/late", func(c *Context) {})
		})
	})

	t.Run("static", func(t *testing.T) {
		e := newSealedEngine()
		expectPanic(t, func() {
			e.Static("/assets", ".")
		})
	})

	t.Run("static file", func(t *testing.T) {
		e := newSealedEngine()
		expectPanic(t, func() {
			e.StaticFile("/favicon.ico", "favicon.ico")
		})
	})

	t.Run("static fs", func(t *testing.T) {
		e := newSealedEngine()
		expectPanic(t, func() {
			e.StaticFS("/public", http.Dir("."))
		})
	})
}

func TestUninitializedEngineReturnsErrors(t *testing.T) {
	var e Engine

	if err := e.Run(); err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if err := e.Shutdown(nil); err == nil {
		t.Fatal("Shutdown() error = nil, want error")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("ServeHTTP status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
