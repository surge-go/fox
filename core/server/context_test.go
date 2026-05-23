package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestContextWithContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gc, _ := gin.CreateTestContext(httptest.NewRecorder())
	gc.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c := &Context{ctx: gc}

	type contextKey string
	key := contextKey("trace")
	ctx := context.WithValue(c.StdContext(), key, "span")

	c.WithContext(ctx)

	if got := c.StdContext().Value(key); got != "span" {
		t.Fatalf("StdContext().Value(%q) = %v, want span", key, got)
	}
}

func TestContextWithContextIgnoresNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gc, _ := gin.CreateTestContext(httptest.NewRecorder())
	gc.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c := &Context{ctx: gc}

	before := c.StdContext()
	c.WithContext(nil)

	if got := c.StdContext(); got != before {
		t.Fatal("WithContext(nil) replaced request context, want unchanged")
	}
}

func TestContextTraceIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gc, _ := gin.CreateTestContext(httptest.NewRecorder())
	gc.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c := &Context{ctx: gc}

	if got := c.TraceID(); got != "" {
		t.Fatalf("TraceID() = %q, want empty", got)
	}
	if got := c.SpanID(); got != "" {
		t.Fatalf("SpanID() = %q, want empty", got)
	}

	c.SetTraceID("trace-1")
	c.SetSpanID("span-1")

	if got := c.TraceID(); got != "trace-1" {
		t.Fatalf("TraceID() = %q, want trace-1", got)
	}
	if got := c.SpanID(); got != "span-1" {
		t.Fatalf("SpanID() = %q, want span-1", got)
	}
}
