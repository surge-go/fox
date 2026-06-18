package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/surge-go/fox"
)

func TestLoggerWritesRequestLine(t *testing.T) {
	log := &memoryLogger{}
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(LoggerWithConfig(LoggerConfig{Logger: log}))
	e.GET("/ping", func(c *fox.Context) {
		c.SetTraceID("trace-1")
		c.String(http.StatusAccepted, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping?name=fox", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	output := log.String()
	for _, want := range []string{
		"[FOX]",
		"GET",
		"202",
		"192.0.2.1",
		"/ping?name=fox",
		"µs",
		"trace-1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("log = %q, want contains %q", output, want)
		}
	}
}

func TestLoggerSkipPaths(t *testing.T) {
	log := &memoryLogger{}
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(LoggerWithConfig(LoggerConfig{
		Logger:    log,
		SkipPaths: []string{"/health"},
	}))
	e.GET("/health", func(c *fox.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if got := log.String(); got != "" {
		t.Fatalf("log = %q, want empty", got)
	}
}

func TestDefaultLogFormatter(t *testing.T) {
	got := DefaultLogFormatter(LogFields{
		Time:     time.Date(2026, 6, 16, 23, 14, 36, 0, time.Local),
		Method:   http.MethodGet,
		Path:     "/ping",
		Status:   http.StatusOK,
		ClientIP: "::1",
		Latency:  464 * time.Microsecond,
		TraceID:  "20260616231436.123456000",
	})

	wantParts := []string{
		"[FOX]",
		"2026/06/16 - 23:14:36",
		"200",
		"464µs",
		"127.0.0.1",
		"GET \"/ping\"",
		"20260616231436.123456000",
	}
	for _, want := range wantParts {
		if !strings.Contains(got, want) {
			t.Fatalf("log = %q, want contains %q", got, want)
		}
	}
}

func TestLoggerCustomFormatter(t *testing.T) {
	log := &memoryLogger{}
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(LoggerWithConfig(LoggerConfig{
		Logger: log,
		Formatter: func(fields LogFields) string {
			return fields.Method + " " + fields.Path + "\n"
		},
	}))
	e.POST("/users", func(c *fox.Context) {
		c.String(http.StatusCreated, "created")
	})

	req := httptest.NewRequest(http.MethodPost, "/users", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if got := log.String(); got != "POST /users\n" {
		t.Fatalf("log = %q, want POST /users", got)
	}
}
