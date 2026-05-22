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
