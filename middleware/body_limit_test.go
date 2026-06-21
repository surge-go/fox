package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/surge-go/fox"
)

func newBodyLimitTestEngine(handler fox.HandlerFunc) *fox.Engine {
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(handler)
	e.POST("/json", func(c *fox.Context) {
		var payload struct {
			Name string `json:"name"`
		}
		if err := c.BindJSON(&payload); err != nil {
			return
		}
		c.Ok(payload)
	})
	return e
}

func TestBodyLimitRejectsKnownLargeContentLength(t *testing.T) {
	e := newBodyLimitTestEngine(BodyLimitWithConfig(BodyLimitConfig{MaxBytes: 8}))

	req := httptest.NewRequest(http.MethodPost, "/json", strings.NewReader(`{"name":"fox"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
	if !strings.Contains(rec.Body.String(), "payload too large") {
		t.Fatalf("body = %q, want payload too large", rec.Body.String())
	}
}

func TestBodyLimitRejectsLargeChunkedBodyOnRead(t *testing.T) {
	e := newBodyLimitTestEngine(BodyLimitWithConfig(BodyLimitConfig{MaxBytes: 8}))

	req := httptest.NewRequest(http.MethodPost, "/json", strings.NewReader(`{"name":"fox"}`))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = -1
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
	if !strings.Contains(rec.Body.String(), "payload too large") {
		t.Fatalf("body = %q, want payload too large", rec.Body.String())
	}
}

func TestBodyLimitAllowsSmallBody(t *testing.T) {
	e := newBodyLimitTestEngine(BodyLimitWithConfig(BodyLimitConfig{MaxBytes: 32}))

	req := httptest.NewRequest(http.MethodPost, "/json", strings.NewReader(`{"name":"fox"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
