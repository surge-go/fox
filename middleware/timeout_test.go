package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/surge-go/fox"
)

func TestTimeoutAllowsFastHandler(t *testing.T) {
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(TimeoutWithConfig(TimeoutConfig{Duration: time.Second}))
	e.GET("/fast", func(c *fox.Context) {
		c.SetHeader("X-Fast", "yes")
		c.String(http.StatusCreated, "fast")
	})

	req := httptest.NewRequest(http.MethodGet, "/fast", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("X-Fast"); got != "yes" {
		t.Fatalf("X-Fast = %q, want yes", got)
	}
	if body := rec.Body.String(); body != "fast" {
		t.Fatalf("body = %q, want fast", body)
	}
}

func TestTimeoutReturnsRequestTimeout(t *testing.T) {
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(TimeoutWithConfig(TimeoutConfig{Duration: 10 * time.Millisecond}))
	e.GET("/slow", func(c *fox.Context) {
		<-c.StdContext().Done()
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestTimeout {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestTimeout)
	}
	if !strings.Contains(rec.Body.String(), "request timeout") {
		t.Fatalf("body = %q, want request timeout", rec.Body.String())
	}
}

func TestTimeoutPreservesBufferedHeadersOnTimeout(t *testing.T) {
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(TimeoutWithConfig(TimeoutConfig{Duration: 10 * time.Millisecond}))
	e.GET("/slow", func(c *fox.Context) {
		c.SetHeader(defaultRequestIDHeader, "req-1")
		<-c.StdContext().Done()
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestTimeout {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestTimeout)
	}
	if got := rec.Header().Get(defaultRequestIDHeader); got != "req-1" {
		t.Fatalf("X-Request-ID = %q, want req-1", got)
	}
}

func TestTimeoutPropagatesPanicToRecovery(t *testing.T) {
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(TimeoutWithConfig(TimeoutConfig{Duration: time.Second}))
	e.GET("/panic", func(c *fox.Context) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
