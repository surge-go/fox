package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/surge-go/fox"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func newTracingTestEngine(handler fox.HandlerFunc) *fox.Engine {
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(handler)
	e.GET("/users/:id", func(c *fox.Context) {
		c.SetHeader("X-Handler-Trace-ID", c.TraceID())
		c.SetHeader("X-Handler-Span-ID", c.SpanID())
		c.SetHeader("X-Handler-Context-Trace-ID", trace.SpanContextFromContext(c.StdContext()).TraceID().String())
		c.Ok(map[string]string{"status": "ok"})
	})
	e.GET("/error", func(c *fox.Context) {
		c.Fail(c.Errors().ErrServer())
	})
	e.GET("/health", func(c *fox.Context) {
		c.String(http.StatusOK, "ok")
	})
	e.GET("/panic", func(c *fox.Context) {
		panic("boom")
	})
	return e
}

func TestTracingCreatesServerSpanAndInjectsContext(t *testing.T) {
	recorder, provider := newTracingTestProvider()
	e := newTracingTestEngine(TracingWithConfig(TracingConfig{
		TracerProvider: provider,
		Propagators:    propagation.TraceContext{},
		RecordHeaders:  []string{"User-Agent", "X-Request-ID"},
	}))

	req := httptest.NewRequest(http.MethodGet, "/users/42?debug=true", nil)
	req.Header.Set("User-Agent", "fox-test")
	req.Header.Set("X-Request-ID", "req-1")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	traceID := rec.Header().Get(defaultTracingResponseTraceHeader)
	if traceID == "" {
		t.Fatal("X-Trace-ID is empty")
	}
	if got := rec.Header().Get("X-Handler-Trace-ID"); got != traceID {
		t.Fatalf("handler trace id = %q, want %q", got, traceID)
	}
	if got := rec.Header().Get("X-Handler-Span-ID"); got == "" {
		t.Fatal("handler span id is empty")
	}
	if got := rec.Header().Get("X-Handler-Context-Trace-ID"); got != traceID {
		t.Fatalf("handler context trace id = %q, want %q", got, traceID)
	}

	spans := tracetest.SpanStubsFromReadOnlySpans(recorder.Ended())
	if len(spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Name != "GET /users/:id" {
		t.Fatalf("span name = %q, want GET /users/:id", span.Name)
	}
	if span.SpanKind != trace.SpanKindServer {
		t.Fatalf("span kind = %s, want server", span.SpanKind)
	}
	if span.SpanContext.TraceID().String() != traceID {
		t.Fatalf("span trace id = %s, want %s", span.SpanContext.TraceID(), traceID)
	}
	assertSpanAttr(t, span.Attributes, "http.request.method", "GET")
	assertSpanAttr(t, span.Attributes, "url.path", "/users/42")
	assertSpanAttr(t, span.Attributes, "http.route", "/users/:id")
	assertSpanAttr(t, span.Attributes, "http.response.status_code", int64(http.StatusOK))
	assertSpanAttr(t, span.Attributes, "user_agent.original", "fox-test")
	assertSpanAttr(t, span.Attributes, "http.request.header.user_agent", []string{"fox-test"})
	assertSpanAttr(t, span.Attributes, "http.request.header.x_request_id", []string{"req-1"})
}

func TestTracingExtractsParentTraceContext(t *testing.T) {
	recorder, provider := newTracingTestProvider()
	e := newTracingTestEngine(TracingWithConfig(TracingConfig{
		TracerProvider: provider,
		Propagators:    propagation.TraceContext{},
	}))

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	spans := tracetest.SpanStubsFromReadOnlySpans(recorder.Ended())
	if len(spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(spans))
	}
	if got := spans[0].Parent.TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("parent trace id = %s", got)
	}
	if got := spans[0].Parent.SpanID().String(); got != "00f067aa0ba902b7" {
		t.Fatalf("parent span id = %s", got)
	}
}

func TestTracingSkipPath(t *testing.T) {
	recorder, provider := newTracingTestProvider()
	e := newTracingTestEngine(TracingWithConfig(TracingConfig{
		TracerProvider: provider,
		SkipPaths:      []string{"/health"},
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get(defaultTracingResponseTraceHeader); got != "" {
		t.Fatalf("X-Trace-ID = %q, want empty", got)
	}
	if spans := recorder.Ended(); len(spans) != 0 {
		t.Fatalf("span count = %d, want 0", len(spans))
	}
}

func TestTracingMarksServerError(t *testing.T) {
	recorder, provider := newTracingTestProvider()
	e := newTracingTestEngine(TracingWithConfig(TracingConfig{TracerProvider: provider}))

	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	spans := tracetest.SpanStubsFromReadOnlySpans(recorder.Ended())
	if len(spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(spans))
	}
	if spans[0].Status.Code != codes.Error {
		t.Fatalf("span status = %v, want error", spans[0].Status.Code)
	}
}

func TestTracingMarksPanicWhenRecoveryIsOuterMiddleware(t *testing.T) {
	recorder, provider := newTracingTestProvider()
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(RecoveryWithConfig(RecoveryConfig{}))
	e.Use(TracingWithConfig(TracingConfig{TracerProvider: provider}))
	e.GET("/panic", func(c *fox.Context) {
		panic(errors.New("boom"))
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	spans := tracetest.SpanStubsFromReadOnlySpans(recorder.Ended())
	if len(spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Status.Code != codes.Error {
		t.Fatalf("span status = %v, want error", span.Status.Code)
	}
	assertSpanAttr(t, span.Attributes, "http.response.status_code", int64(http.StatusInternalServerError))
	if len(span.Events) == 0 {
		t.Fatal("span events is empty, want recorded panic error")
	}
}

func TestTracingUsesCurrentGlobalProviderWhenProviderIsNil(t *testing.T) {
	recorder, provider := newTracingTestProvider()
	previous := otel.GetTracerProvider()
	defer otel.SetTracerProvider(previous)

	handler := Tracing()
	otel.SetTracerProvider(provider)
	e := newTracingTestEngine(handler)

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if spans := recorder.Ended(); len(spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(spans))
	}
}

func TestTracingResponseTraceHeaderOptions(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		_, provider := newTracingTestProvider()
		e := newTracingTestEngine(TracingWithConfig(TracingConfig{
			TracerProvider:             provider,
			DisableResponseTraceHeader: true,
		}))

		req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if got := rec.Header().Get(defaultTracingResponseTraceHeader); got != "" {
			t.Fatalf("X-Trace-ID = %q, want empty", got)
		}
		if got := rec.Header().Get("X-Handler-Trace-ID"); got == "" {
			t.Fatal("handler trace id is empty")
		}
	})

	t.Run("custom", func(t *testing.T) {
		_, provider := newTracingTestProvider()
		e := newTracingTestEngine(TracingWithConfig(TracingConfig{
			TracerProvider:      provider,
			ResponseTraceHeader: "X-Request-Trace",
		}))

		req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if got := rec.Header().Get(defaultTracingResponseTraceHeader); got != "" {
			t.Fatalf("X-Trace-ID = %q, want empty", got)
		}
		if got := rec.Header().Get("X-Request-Trace"); got == "" {
			t.Fatal("X-Request-Trace is empty")
		}
	})
}

func TestTracingSkipFunc(t *testing.T) {
	recorder, provider := newTracingTestProvider()
	e := newTracingTestEngine(TracingWithConfig(TracingConfig{
		TracerProvider: provider,
		SkipFunc: func(c *fox.Context) bool {
			return c.RawRequest().URL.Path == "/health"
		},
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if spans := recorder.Ended(); len(spans) != 0 {
		t.Fatalf("span count = %d, want 0", len(spans))
	}
}

func TestTracingSkipsSensitiveRecordHeaders(t *testing.T) {
	recorder, provider := newTracingTestProvider()
	e := newTracingTestEngine(TracingWithConfig(TracingConfig{
		TracerProvider: provider,
		RecordHeaders:  []string{"Authorization", "Cookie", "Set-Cookie", "Proxy-Authorization", "X-API-Key", "X-Request-ID"},
	}))

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Cookie", "sid=secret")
	req.Header.Set("Set-Cookie", "sid=secret")
	req.Header.Set("Proxy-Authorization", "Basic secret")
	req.Header.Set("X-API-Key", "secret")
	req.Header.Set("X-Request-ID", "req-1")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	spans := tracetest.SpanStubsFromReadOnlySpans(recorder.Ended())
	if len(spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(spans))
	}
	assertSpanAttr(t, spans[0].Attributes, "http.request.header.x_request_id", []string{"req-1"})
	assertSpanAttrMissing(t, spans[0].Attributes, "http.request.header.authorization")
	assertSpanAttrMissing(t, spans[0].Attributes, "http.request.header.cookie")
	assertSpanAttrMissing(t, spans[0].Attributes, "http.request.header.set_cookie")
	assertSpanAttrMissing(t, spans[0].Attributes, "http.request.header.proxy_authorization")
	assertSpanAttrMissing(t, spans[0].Attributes, "http.request.header.x_api_key")
}

func newTracingTestProvider() (*tracetest.SpanRecorder, *sdktrace.TracerProvider) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	return recorder, provider
}

func assertSpanAttrMissing(t *testing.T, attrs []attribute.KeyValue, key string) {
	t.Helper()
	for _, attr := range attrs {
		if string(attr.Key) == key {
			t.Fatalf("attribute %s should be absent, got %#v", key, attr.Value.AsInterface())
		}
	}
}

func assertSpanAttr(t *testing.T, attrs []attribute.KeyValue, key string, want any) {
	t.Helper()
	for _, attr := range attrs {
		if string(attr.Key) != key {
			continue
		}
		switch value := want.(type) {
		case string:
			if got := attr.Value.AsString(); got != value {
				t.Fatalf("attribute %s = %q, want %q", key, got, value)
			}
		case int64:
			if got := attr.Value.AsInt64(); got != value {
				t.Fatalf("attribute %s = %d, want %d", key, got, value)
			}
		case []string:
			got := attr.Value.AsStringSlice()
			if len(got) != len(value) {
				t.Fatalf("attribute %s = %#v, want %#v", key, got, value)
			}
			for i := range value {
				if got[i] != value[i] {
					t.Fatalf("attribute %s = %#v, want %#v", key, got, value)
				}
			}
		default:
			t.Fatalf("unsupported attribute assertion type %T", want)
		}
		return
	}
	t.Fatalf("attribute %s not found in %#v", key, attrs)
}
