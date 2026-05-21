package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	foxtracing "github.com/surge-go/fox/core/tracing"
)

func TestNewRegistersTracingMiddlewareWhenGlobalProviderExists(t *testing.T) {
	provider, err := foxtracing.New(context.Background(), &foxtracing.Config{
		Exporter: foxtracing.ExporterNone,
	})
	if err != nil {
		t.Fatalf("failed to create tracer provider: %v", err)
	}
	defer provider.Shutdown(context.Background())

	engine, err := New(&Config{
		Mode: ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	traceID := ""
	engine.GET("/users/:id", func(c *Context) {
		traceID = c.TraceID()
		c.String(http.StatusOK, c.Param("id"))
	})

	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if traceID == "" {
		t.Fatal("expected built-in tracing middleware to set trace_id")
	}
}

func TestNewSkipsTracingMiddlewareAfterProviderShutdown(t *testing.T) {
	provider, err := foxtracing.New(context.Background(), &foxtracing.Config{
		Exporter: foxtracing.ExporterNone,
	})
	if err != nil {
		t.Fatalf("failed to create tracer provider: %v", err)
	}
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("failed to shutdown tracer provider: %v", err)
	}

	engine, err := New(&Config{
		Mode: ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	traceID := "unexpected"
	engine.GET("/no-tracing", func(c *Context) {
		traceID = c.TraceID()
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/no-tracing", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if traceID != "" {
		t.Fatalf("expected trace_id to be empty after provider shutdown, got %q", traceID)
	}
}
