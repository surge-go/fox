package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/surge-go/fox/core/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestTracingCreatesServerSpan(t *testing.T) {
	recorder, shutdown := setupTracingTest(t)
	defer shutdown()

	engine := newTestEngine(t)
	handlerSawSpan := false
	engine.Use(Tracing())
	engine.GET("/users/:id", func(c *server.Context) {
		handlerSawSpan = trace.SpanFromContext(c.StdContext()).SpanContext().IsValid()
		c.Ok(map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !handlerSawSpan {
		t.Fatal("handler did not see tracing span in server context")
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans len = %d, want 1", len(spans))
	}
	span := spans[0]
	if got, want := span.Name(), "GET /users/:id"; got != want {
		t.Fatalf("span name = %q, want %q", got, want)
	}
	if got, want := span.SpanKind(), trace.SpanKindServer; got != want {
		t.Fatalf("span kind = %v, want %v", got, want)
	}
	if got, want := span.Parent().SpanID().String(), "00f067aa0ba902b7"; got != want {
		t.Fatalf("parent span id = %q, want %q", got, want)
	}

	attrs := spanAttrs(span.Attributes())
	assertStringAttr(t, attrs, "http.request.method", "GET")
	assertStringAttr(t, attrs, "url.path", "/users/123")
	assertStringAttr(t, attrs, "http.route", "/users/:id")
	assertIntAttr(t, attrs, "http.response.status_code", http.StatusOK)
}

func TestTracingRecordsConfiguredHeadersAndQuery(t *testing.T) {
	recorder, shutdown := setupTracingTest(t)
	defer shutdown()

	engine := newTestEngine(t)
	engine.Use(Tracing(TracingConfig{
		RecordQuery:           true,
		RecordRequestHeaders:  []string{"X-Request-ID"},
		RecordResponseHeaders: []string{"X-Trace-ID"},
	}))
	engine.GET("/headers", func(c *server.Context) {
		c.SetHeader("X-Trace-ID", "trace-1")
		c.Ok(map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/headers?page=1", nil)
	req.Header.Set("X-Request-ID", "req-1")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans len = %d, want 1", len(spans))
	}
	attrs := spanAttrs(spans[0].Attributes())
	assertStringAttr(t, attrs, "url.query", "page=1")
	assertStringSliceAttr(t, attrs, "http.request.header.x-request-id", []string{"req-1"})
	assertStringSliceAttr(t, attrs, "http.response.header.x-trace-id", []string{"trace-1"})
}

func TestTracingSkipFunc(t *testing.T) {
	recorder, shutdown := setupTracingTest(t)
	defer shutdown()

	engine := newTestEngine(t)
	handlerSawSpan := true
	engine.Use(Tracing(TracingConfig{
		SkipFunc: func(c *server.Context) bool {
			return c.RawRequest().URL.Path == "/health"
		},
	}))
	engine.GET("/health", func(c *server.Context) {
		handlerSawSpan = trace.SpanFromContext(c.StdContext()).SpanContext().IsValid()
		c.Ok(map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if handlerSawSpan {
		t.Fatal("handler saw span for skipped request")
	}
	if spans := recorder.Ended(); len(spans) != 0 {
		t.Fatalf("ended spans len = %d, want 0", len(spans))
	}
}

func TestTracingMarksServerError(t *testing.T) {
	recorder, shutdown := setupTracingTest(t)
	defer shutdown()

	engine := newTestEngine(t)
	engine.Use(Tracing())
	engine.GET("/fail", func(c *server.Context) {
		c.AbortWithStatus(http.StatusInternalServerError)
	})

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans len = %d, want 1", len(spans))
	}
	if got, want := spans[0].Status().Code, codes.Error; got != want {
		t.Fatalf("span status = %v, want %v", got, want)
	}
}

func TestTracingFinishesSpanOnPanic(t *testing.T) {
	recorder, shutdown := setupTracingTest(t)
	defer shutdown()

	engine := newTestEngine(t)
	engine.Use(Tracing())
	engine.GET("/panic", func(c *server.Context) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans len = %d, want 1", len(spans))
	}
	if got, want := spans[0].Name(), "GET /panic"; got != want {
		t.Fatalf("span name = %q, want %q", got, want)
	}
	if got, want := spans[0].Status().Code, codes.Error; got != want {
		t.Fatalf("span status = %v, want %v", got, want)
	}
	attrs := spanAttrs(spans[0].Attributes())
	assertIntAttr(t, attrs, "http.response.status_code", http.StatusInternalServerError)
}

func newTestEngine(t *testing.T) *server.Engine {
	t.Helper()
	engine, err := server.New(&server.Config{Addr: ":8080", Mode: server.ModeTest})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	return engine
}

func setupTracingTest(t *testing.T) (*tracetest.SpanRecorder, func()) {
	t.Helper()

	oldProvider := otel.GetTracerProvider()
	oldPropagator := otel.GetTextMapPropagator()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return recorder, func() {
		_ = provider.Shutdown(context.Background())
		if oldProvider != nil {
			otel.SetTracerProvider(oldProvider)
		} else {
			otel.SetTracerProvider(noop.NewTracerProvider())
		}
		otel.SetTextMapPropagator(oldPropagator)
	}
}

func spanAttrs(attrs []attribute.KeyValue) map[string]attribute.Value {
	values := make(map[string]attribute.Value, len(attrs))
	for _, attr := range attrs {
		values[string(attr.Key)] = attr.Value
	}
	return values
}

func assertStringAttr(t *testing.T, attrs map[string]attribute.Value, key, want string) {
	t.Helper()
	got, ok := attrs[key]
	if !ok {
		t.Fatalf("missing attribute %q", key)
	}
	if got.AsString() != want {
		t.Fatalf("attribute %q = %q, want %q", key, got.AsString(), want)
	}
}

func assertIntAttr(t *testing.T, attrs map[string]attribute.Value, key string, want int) {
	t.Helper()
	got, ok := attrs[key]
	if !ok {
		t.Fatalf("missing attribute %q", key)
	}
	if got.AsInt64() != int64(want) {
		t.Fatalf("attribute %q = %d, want %d", key, got.AsInt64(), want)
	}
}

func assertStringSliceAttr(t *testing.T, attrs map[string]attribute.Value, key string, want []string) {
	t.Helper()
	got, ok := attrs[key]
	if !ok {
		t.Fatalf("missing attribute %q", key)
	}
	gotSlice := got.AsStringSlice()
	if len(gotSlice) != len(want) {
		t.Fatalf("attribute %q len = %d, want %d", key, len(gotSlice), len(want))
	}
	for i := range want {
		if gotSlice[i] != want[i] {
			t.Fatalf("attribute %q[%d] = %q, want %q", key, i, gotSlice[i], want[i])
		}
	}
}
