package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNewRegistersRecoveryMiddleware(t *testing.T) {
	engine, err := New(&Config{
		Mode: ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.GET("/panic", func(c *Context) {
		panic("test panic")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}
}

func TestRecoveryHidesPanicDetailsInReleaseMode(t *testing.T) {
	engine, err := New(&Config{
		Mode: ModeRelease,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.GET("/panic", func(c *Context) {
		panic("secret panic detail")
	})

	output := captureStdout(t, func() {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/panic", nil)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}
	})

	if !strings.Contains(output, "[Recovery] panic recovered") {
		t.Fatalf("expected recovery output, got %q", output)
	}
	if strings.Contains(output, "secret panic detail") || strings.Contains(output, "goroutine ") {
		t.Fatalf("expected release recovery output to hide panic details and stack, got %q", output)
	}
}

func TestNewLoggerRecordsPanicRequest(t *testing.T) {
	engine, err := New(&Config{
		Mode: ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.GET("/panic", func(c *Context) {
		panic("test panic")
	})

	output := captureStdout(t, func() {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/panic", nil)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}
	})

	if !strings.Contains(output, "[Recovery] panic recovered") {
		t.Fatalf("expected recovery output, got %q", output)
	}
	if !strings.Contains(output, "[FOX]") {
		t.Fatalf("expected built-in logger output, got %q", output)
	}
	if !strings.Contains(output, `500`) || !strings.Contains(output, `GET     "/panic"`) {
		t.Fatalf("expected panic access log with status 500, got %q", output)
	}
}

func TestNewRegistersLoggerMiddlewareOutsideReleaseByDefault(t *testing.T) {
	engine, err := New(&Config{
		Mode: ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.GET("/ping", func(c *Context) {
		c.String(http.StatusOK, "pong")
	})

	output := captureStdout(t, func() {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		engine.ServeHTTP(w, req)
	})

	if !strings.Contains(output, "[FOX]") {
		t.Fatalf("expected built-in logger output, got %q", output)
	}
	if !strings.Contains(output, `GET     "/ping"`) {
		t.Fatalf("expected request line in logger output, got %q", output)
	}
}

func TestNewLoggerRecordsTraceContextFields(t *testing.T) {
	engine, err := New(&Config{
		Mode: ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.Use(func(c *Context) {
		c.Set(TraceIDKey, "4bf92f3577b34da6a3ce929d0e0e4736")
		c.Set(SpanIDKey, "00f067aa0ba902b7")
		c.Next()
	})
	engine.GET("/ping", func(c *Context) {
		c.String(http.StatusOK, "pong")
	})

	output := captureStdout(t, func() {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		engine.ServeHTTP(w, req)
	})

	if !strings.Contains(output, "trace_id=4bf92f3577b34da6a3ce929d0e0e4736") {
		t.Fatalf("expected trace id in logger output, got %q", output)
	}
	if !strings.Contains(output, "span_id=00f067aa0ba902b7") {
		t.Fatalf("expected span id in logger output, got %q", output)
	}
}

func TestNewSkipsLoggerMiddlewareInReleaseByDefault(t *testing.T) {
	engine, err := New(&Config{
		Mode: ModeRelease,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.GET("/ping", func(c *Context) {
		c.String(http.StatusOK, "pong")
	})

	output := captureStdout(t, func() {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		engine.ServeHTTP(w, req)
	})

	if output != "" {
		t.Fatalf("expected no built-in logger output in release mode, got %q", output)
	}
}

func TestNewSkipsLoggerMiddlewareWhenDisabled(t *testing.T) {
	enableLogger := false
	engine, err := New(&Config{
		Mode:         ModeDebug,
		Addr:         ":8080",
		EnableLogger: &enableLogger,
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.GET("/ping", func(c *Context) {
		c.String(http.StatusOK, "pong")
	})

	output := captureStdout(t, func() {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		engine.ServeHTTP(w, req)
	})

	if output != "" {
		t.Fatalf("expected no built-in logger output when disabled, got %q", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	os.Stdout = writer
	defer func() {
		os.Stdout = oldStdout
	}()

	out := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(reader)
		out <- string(data)
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close stdout pipe: %v", err)
	}
	return <-out
}
