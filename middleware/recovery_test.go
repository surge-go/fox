package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/surge-go/fox"
)

func TestRecoveryReturnsJSON500(t *testing.T) {
	log := &memoryLogger{}
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(RecoveryWithConfig(RecoveryConfig{Logger: log, EnableStack: true}))
	e.GET("/panic", func(c *fox.Context) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	var resp fox.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Code != http.StatusInternalServerError || resp.Message != "internal server error" {
		t.Fatalf("response = %+v, want 500 internal server error", resp)
	}
	if output := log.String(); !strings.Contains(output, "boom") || !strings.Contains(output, "goroutine") {
		t.Fatalf("log = %q, want panic and stack", output)
	}
}

func TestRecoveryDoesNotRewriteWrittenResponse(t *testing.T) {
	log := &memoryLogger{}
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(RecoveryWithConfig(RecoveryConfig{Logger: log}))
	e.GET("/panic-after-write", func(c *fox.Context) {
		c.String(http.StatusCreated, "created")
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic-after-write", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if body := rec.Body.String(); body != "created" {
		t.Fatalf("body = %q, want created", body)
	}
	if contentType := rec.Header().Get("Content-Type"); strings.Contains(contentType, "application/json") {
		t.Fatalf("content-type = %q, did not expect JSON rewrite", contentType)
	}
	if output := log.String(); !strings.Contains(output, "boom") {
		t.Fatalf("log = %q, want panic", output)
	}
}
