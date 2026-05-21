package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/surge-go/fox/core/server"
)

func TestCORS_DefaultConfig(t *testing.T) {
	cfg := &server.Config{Addr: ":8080", Mode: server.ModeTest}
	engine, err := server.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.Use(CORS(nil))
	engine.GET("/test", func(c *server.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// 默认配置允许所有源
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected Access-Control-Allow-Origin: *, got %s", got)
	}
}

func TestCORS_PreflightRequest(t *testing.T) {
	cfg := &server.Config{Addr: ":8080", Mode: server.ModeTest}
	engine, err := server.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.Use(CORS(&CORSConfig{
		AllowOrigins:     []string{"https://example.com"},
		AllowMethods:     []string{"GET", "POST"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           3600,
	}))
	engine.POST("/test", func(c *server.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})

	// 预检请求
	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Errorf("expected Access-Control-Allow-Origin: https://example.com, got %s", got)
	}

	if got := w.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST" {
		t.Errorf("expected Access-Control-Allow-Methods: GET, POST, got %s", got)
	}

	if got := w.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization" {
		t.Errorf("expected Access-Control-Allow-Headers: Content-Type, Authorization, got %s", got)
	}

	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("expected Access-Control-Allow-Credentials: true, got %s", got)
	}

	if got := w.Header().Get("Access-Control-Max-Age"); got != "3600" {
		t.Errorf("expected Access-Control-Max-Age: 3600, got %s", got)
	}
}

func TestCORS_ActualRequest(t *testing.T) {
	cfg := &server.Config{Addr: ":8080", Mode: server.ModeTest}
	engine, err := server.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.Use(CORS(&CORSConfig{
		AllowOrigins:     []string{"https://example.com"},
		AllowCredentials: true,
		ExposeHeaders:    []string{"X-Request-ID"},
	}))
	engine.GET("/test", func(c *server.Context) {
		c.SetHeader("X-Request-ID", "12345")
		c.Ok(map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Errorf("expected Access-Control-Allow-Origin: https://example.com, got %s", got)
	}

	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("expected Access-Control-Allow-Credentials: true, got %s", got)
	}

	if got := w.Header().Get("Access-Control-Expose-Headers"); got != "X-Request-ID" {
		t.Errorf("expected Access-Control-Expose-Headers: X-Request-ID, got %s", got)
	}
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	cfg := &server.Config{Addr: ":8080", Mode: server.ModeTest}
	engine, err := server.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.Use(CORS(&CORSConfig{
		AllowOrigins: []string{"https://example.com"},
	}))
	engine.GET("/test", func(c *server.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// 不允许的源不应该设置 CORS 头
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Access-Control-Allow-Origin header, got %s", got)
	}
}

func TestCORS_WildcardSubdomain(t *testing.T) {
	cfg := &server.Config{Addr: ":8080", Mode: server.ModeTest}
	engine, err := server.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.Use(CORS(&CORSConfig{
		AllowOrigins: []string{"*.example.com"},
	}))
	engine.GET("/test", func(c *server.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})

	// 测试匹配的子域名
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("expected Access-Control-Allow-Origin: https://app.example.com, got %s", got)
	}

	// 测试不匹配的域名
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Access-Control-Allow-Origin header, got %s", got)
	}

	// 测试伪子域名不应该被后缀匹配放行
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://badexample.com")
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Access-Control-Allow-Origin header for spoofed domain, got %s", got)
	}

	// 通配符只匹配子域名，不匹配根域名
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Access-Control-Allow-Origin header for root domain, got %s", got)
	}
}

func TestCORS_WildcardOriginWithCredentialsReflectsOrigin(t *testing.T) {
	cfg := &server.Config{Addr: ":8080", Mode: server.ModeTest}
	engine, err := server.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.Use(CORS(&CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowCredentials: true,
	}))
	engine.GET("/test", func(c *server.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("expected Access-Control-Allow-Origin to reflect origin, got %s", got)
	}

	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("expected Access-Control-Allow-Credentials: true, got %s", got)
	}

	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("expected Vary: Origin, got %s", got)
	}
}

func TestCORS_NoOriginHeader(t *testing.T) {
	cfg := &server.Config{Addr: ":8080", Mode: server.ModeTest}
	engine, err := server.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.Use(CORS(&CORSConfig{
		AllowOrigins: []string{"https://example.com"},
	}))
	engine.GET("/test", func(c *server.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})

	// 没有 Origin 头的请求（同源请求）
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// 不应该设置 CORS 头
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Access-Control-Allow-Origin header, got %s", got)
	}
}
