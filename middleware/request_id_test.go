package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/surge-go/fox"
)

func TestRequestIDUsesIncomingHeader(t *testing.T) {
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(RequestID())
	e.GET("/test", func(c *fox.Context) {
		c.SetHeader("X-Handler-Request-ID", c.RequestID())
		c.Ok(map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(defaultRequestIDHeader, "req-1")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get(defaultRequestIDHeader); got != "req-1" {
		t.Fatalf("X-Request-ID = %q, want req-1", got)
	}
	if got := rec.Header().Get("X-Handler-Request-ID"); got != "req-1" {
		t.Fatalf("handler request id = %q, want req-1", got)
	}
}

func TestRequestIDGeneratesWhenMissing(t *testing.T) {
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(RequestIDWithConfig(RequestIDConfig{
		Generator: func(*fox.Context) string {
			return "generated-1"
		},
	}))
	e.GET("/test", func(c *fox.Context) {
		c.String(http.StatusOK, c.RequestID())
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get(defaultRequestIDHeader); got != "generated-1" {
		t.Fatalf("X-Request-ID = %q, want generated-1", got)
	}
	if body := rec.Body.String(); body != "generated-1" {
		t.Fatalf("body = %q, want generated-1", body)
	}
}

func TestRequestIDCanIgnoreIncomingHeader(t *testing.T) {
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(RequestIDWithConfig(RequestIDConfig{
		IgnoreHeader: true,
		Generator: func(*fox.Context) string {
			return "generated-1"
		},
	}))
	e.GET("/test", func(c *fox.Context) {
		c.String(http.StatusOK, c.RequestID())
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(defaultRequestIDHeader, "client-1")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get(defaultRequestIDHeader); got != "generated-1" {
		t.Fatalf("X-Request-ID = %q, want generated-1", got)
	}
}

func TestLoggerPrintsRequestIDAndDedupesTraceID(t *testing.T) {
	log := &memoryLogger{}
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(func(c *fox.Context) {
		c.SetRequestID("same-id")
		c.SetTraceID("same-id")
		c.Next()
	})
	e.Use(LoggerWithConfig(LoggerConfig{Logger: log}))
	e.GET("/test", func(c *fox.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	line := log.String()
	if strings.Count(line, "same-id") != 1 {
		t.Fatalf("log line = %q, want same-id once", line)
	}
}
