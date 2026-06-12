package server

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	foxerrors "github.com/surge-go/fox/core/errors"
	"github.com/surge-go/fox/pkg/openapi"
)

func TestNewDefaultsModeAndNormalizesBarePort(t *testing.T) {
	engine, err := New(&Config{Addr: "8080"})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	if engine.mode != ModeRelease {
		t.Fatalf("expected default mode %q, got %q", ModeRelease, engine.mode)
	}
	if engine.server.Addr != ":8080" {
		t.Fatalf("expected normalized addr :8080, got %q", engine.server.Addr)
	}
}

func TestClientIPDoesNotTrustProxyHeadersByDefault(t *testing.T) {
	engine, err := New(&Config{
		Mode: ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.GET("/ip", func(c *Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if got := w.Body.String(); got != "192.0.2.1" {
		t.Fatalf("expected remote addr client IP, got %q", got)
	}
}

func TestClientIPTrustsConfiguredProxyHeaders(t *testing.T) {
	engine, err := New(&Config{
		Mode:           ModeTest,
		Addr:           ":8080",
		TrustedProxies: []string{"192.0.2.1"},
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.GET("/ip", func(c *Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if got := w.Body.String(); got != "203.0.113.9" {
		t.Fatalf("expected forwarded client IP, got %q", got)
	}
}

func TestFailUsesHTTPStatusAndPublicMessage(t *testing.T) {
	engine, err := New(&Config{
		Mode: ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.GET("/fail", func(c *Context) {
		c.Fail(foxerrors.NewWithStatus(10001, http.StatusBadRequest, "bad request").WithErr(stderrors.New("secret database detail")))
	})

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Code != 10001 {
		t.Fatalf("expected business code 10001, got %d", resp.Code)
	}
	if resp.Message != "bad request" {
		t.Fatalf("expected public message, got %q", resp.Message)
	}
}

func TestFailFallsBackWhenErrorStatusIsInvalid(t *testing.T) {
	engine, err := New(&Config{
		Mode: ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.GET("/fail", func(c *Context) {
		c.Fail(foxerrors.NewWithStatus(10001, 42, "bad status"))
	})

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}
}

func TestAdvancedResponsesIncludeTraceID(t *testing.T) {
	enableLogger := false
	engine, err := New(&Config{
		Mode:         ModeTest,
		Addr:         ":8080",
		EnableLogger: &enableLogger,
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.Use(func(c *Context) {
		c.SetTraceID("trace-1")
		c.Next()
	})
	engine.GET("/ok", func(c *Context) {
		c.Ok(map[string]string{"status": "ok"})
	})
	engine.GET("/fail", func(c *Context) {
		c.Fail(foxerrors.NewWithStatus(10001, http.StatusBadRequest, "bad request"))
	})
	engine.POST("/bind", func(c *Context) {
		var req struct {
			Name string `json:"name" binding:"required"`
		}
		if err := c.BindJSON(&req); err != nil {
			return
		}
		c.Ok(nil)
	})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantCode   int
	}{
		{
			name:       "ok",
			method:     http.MethodGet,
			path:       "/ok",
			wantStatus: http.StatusOK,
			wantCode:   http.StatusOK,
		},
		{
			name:       "fail",
			method:     http.MethodGet,
			path:       "/fail",
			wantStatus: http.StatusBadRequest,
			wantCode:   10001,
		},
		{
			name:       "bind_error",
			method:     http.MethodPost,
			path:       "/bind",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()

			engine.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, w.Code)
			}

			var resp Response
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if resp.Code != tt.wantCode {
				t.Fatalf("response code = %d, want %d", resp.Code, tt.wantCode)
			}
			if resp.TraceID != "trace-1" {
				t.Fatalf("response trace id = %q, want trace-1", resp.TraceID)
			}
		})
	}
}

func TestRouterGroupMiddlewareIsolation(t *testing.T) {
	engine, err := New(&Config{
		Mode: ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	middlewareA := func(c *Context) {
		c.Set("marker", "a")
		c.Next()
	}
	middlewareB := func(c *Context) {
		c.Set("marker", "b")
		c.Next()
	}

	group := engine.Group("/api", middlewareA)
	child := group.Group("/v1")
	group.Use(middlewareB)

	child.GET("/marker", func(c *Context) {
		c.String(http.StatusOK, c.GetString("marker"))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/marker", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got := w.Body.String(); got != "a" {
		t.Fatalf("expected child group to keep original middleware, got %q", got)
	}
}

func TestAnyRecordsEachHTTPMethod(t *testing.T) {
	engine, err := New(&Config{
		Mode: ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.Any("/any", func(c *Context) {})

	routes := engine.routeSnapshot()
	if got, want := len(routes), len(anyMethods); got != want {
		t.Fatalf("expected %d route records, got %d", want, got)
	}

	for i, method := range anyMethods {
		if routes[i].Method != method {
			t.Fatalf("route %d method = %q, want %q", i, routes[i].Method, method)
		}
		if routes[i].Path != "/any" {
			t.Fatalf("route %d path = %q, want %q", i, routes[i].Path, "/any")
		}
	}
}

func TestRouteHideFromRouteList(t *testing.T) {
	engine, err := New(&Config{
		Mode: ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.POST("/internal/proxy", func(c *Context) {
		c.String(http.StatusOK, "ok")
	}).HideFromRouteList()

	routes := engine.routeSnapshot()
	if got, want := len(routes), 1; got != want {
		t.Fatalf("route records = %d, want %d", got, want)
	}
	if !routes[0].Hidden {
		t.Fatal("route Hidden = false, want true")
	}

	req := httptest.NewRequest(http.MethodPost, "/internal/proxy", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("hidden route status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestPrintRoutesIncludesOpenAPIURLsWhenConfigured(t *testing.T) {
	engine, err := New(&Config{
		Mode: ModeTest,
		Addr: ":8080",
		OpenAPI: &OpenAPIConfig{
			Info: openapi.Info{Title: "API", Version: "1.0.0"},
		},
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.GET("/ping", func(c *Context) {
		c.String(http.StatusOK, "pong")
	})

	output := captureStdout(t, func() {
		engine.printRoutes()
	})

	if !strings.Contains(output, "/ ____/___") || !strings.Contains(output, "Fox Web 框架") || !strings.Contains(output, "运行模式") || !strings.Contains(output, "test") || !strings.Contains(output, "监听地址") || !strings.Contains(output, "http://localhost:8080") {
		t.Fatalf("expected startup banner in route output, got %q", output)
	}
	if !strings.Contains(output, "OpenAPI UI:") || !strings.Contains(output, "http://localhost:8080/docs") {
		t.Fatalf("expected OpenAPI UI URL in route output, got %q", output)
	}
	if !strings.Contains(output, "OpenAPI JSON:") || !strings.Contains(output, "http://localhost:8080/openapi.json") {
		t.Fatalf("expected OpenAPI JSON URL in route output, got %q", output)
	}
}

func TestGetHandlerNameHandlesNilHandler(t *testing.T) {
	engine, err := New(&Config{
		Mode: ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	if got := engine.getHandlerName(nil); got != "<nil>" {
		t.Fatalf("expected <nil>, got %q", got)
	}
}

func TestConcurrentRouteRegistration(t *testing.T) {
	engine, err := New(&Config{
		Mode: ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	group := engine.Group("/api", func(c *Context) {})

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		i := i
		wg.Add(2)

		go func() {
			defer wg.Done()
			engine.GET(fmt.Sprintf("/direct/%d", i), func(c *Context) {})
		}()

		go func() {
			defer wg.Done()
			group.GET(fmt.Sprintf("/group/%d", i), func(c *Context) {})
		}()
	}

	wg.Wait()

	if got, want := len(engine.routeSnapshot()), 64; got != want {
		t.Fatalf("expected %d route records, got %d", want, got)
	}
}
