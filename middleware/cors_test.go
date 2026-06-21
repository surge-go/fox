package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/surge-go/fox"
)

func newCORSTestEngine(handler fox.HandlerFunc) *fox.Engine {
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(handler)
	e.GET("/test", func(c *fox.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})
	e.POST("/test", func(c *fox.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})
	return e
}

func boolPtr(v bool) *bool {
	return &v
}

func TestCORSDefaultConfigAllowsAnyOrigin(t *testing.T) {
	e := newCORSTestEngine(CORS())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want *", got)
	}
}

func TestCORSPreflightRequest(t *testing.T) {
	e := newCORSTestEngine(CORSWithConfig(CORSConfig{
		AllowOrigins:     []string{"https://example.com"},
		AllowMethods:     []string{http.MethodGet, http.MethodPost},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           time.Hour,
	}))

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want https://example.com", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST" {
		t.Fatalf("Access-Control-Allow-Methods = %q, want GET, POST", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization" {
		t.Fatalf("Access-Control-Allow-Headers = %q, want Content-Type, Authorization", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Access-Control-Allow-Credentials = %q, want true", got)
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got != "3600" {
		t.Fatalf("Access-Control-Max-Age = %q, want 3600", got)
	}
}

func TestCORSDoesNotInterceptPlainOptionsRequest(t *testing.T) {
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(CORS())
	e.OPTIONS("/custom", func(c *fox.Context) {
		c.String(http.StatusAccepted, "custom")
	})

	req := httptest.NewRequest(http.MethodOptions, "/custom", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if body := rec.Body.String(); body != "custom" {
		t.Fatalf("body = %q, want custom", body)
	}
}

func TestCORSActualRequest(t *testing.T) {
	e := newCORSTestEngine(CORSWithConfig(CORSConfig{
		AllowOrigins:     []string{"https://example.com"},
		AllowCredentials: true,
		ExposeHeaders:    []string{"X-Request-ID"},
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want https://example.com", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Access-Control-Allow-Credentials = %q, want true", got)
	}
	if got := rec.Header().Get("Access-Control-Expose-Headers"); got != "X-Request-ID" {
		t.Fatalf("Access-Control-Expose-Headers = %q, want X-Request-ID", got)
	}
}

func TestCORSDisallowedOrigin(t *testing.T) {
	e := newCORSTestEngine(CORSWithConfig(CORSConfig{
		AllowOrigins: []string{"https://example.com"},
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestCORSWildcardSubdomain(t *testing.T) {
	e := newCORSTestEngine(CORSWithConfig(CORSConfig{
		AllowOrigins: []string{"*.example.com"},
	}))

	tests := []struct {
		name      string
		origin    string
		wantAllow bool
	}{
		{name: "subdomain", origin: "https://app.example.com", wantAllow: true},
		{name: "evil", origin: "https://evil.com", wantAllow: false},
		{name: "spoofed suffix", origin: "https://badexample.com", wantAllow: false},
		{name: "root domain", origin: "https://example.com", wantAllow: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Origin", tt.origin)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			got := rec.Header().Get("Access-Control-Allow-Origin")
			if tt.wantAllow && got != tt.origin {
				t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, tt.origin)
			}
			if !tt.wantAllow && got != "" {
				t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
			}
		})
	}
}

func TestCORSWildcardOriginWithCredentialsReflectsOrigin(t *testing.T) {
	e := newCORSTestEngine(CORSWithConfig(CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowCredentials: true,
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want reflected origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Access-Control-Allow-Credentials = %q, want true", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q, want Origin", got)
	}
}

func TestCORSNoOriginHeader(t *testing.T) {
	e := newCORSTestEngine(CORSWithConfig(CORSConfig{
		AllowOrigins: []string{"https://example.com"},
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestCORSConfigIsCopied(t *testing.T) {
	cfg := CORSConfig{
		AllowOrigins: []string{"https://example.com"},
	}
	handler := CORSWithConfig(cfg)
	cfg.AllowOrigins[0] = "https://evil.com"
	e := newCORSTestEngine(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want https://example.com", got)
	}
}
