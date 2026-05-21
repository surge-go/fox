package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/surge-go/fox/core/server"
	"github.com/surge-go/fox/core/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestTracing(t *testing.T) {
	// 创建内存 exporter 用于测试
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	defer provider.Shutdown(context.Background())

	// 设置全局 propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// 创建测试服务器
	engine, err := server.New(&server.Config{
		Mode: server.ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	engine.Use(Tracing(provider))
	engine.GET("/users/:id", func(c *server.Context) {
		// 验证 trace_id 已注入
		traceID := c.TraceID()
		if traceID == "" {
			t.Error("trace_id not set in context")
		}
		c.JSON(http.StatusOK, map[string]string{"id": c.Param("id")})
	})

	// 发送测试请求
	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	// 验证响应
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// 验证 span 已创建
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	if span.Name != "GET /users/:id" {
		t.Errorf("expected span name 'GET /users/:id', got '%s'", span.Name)
	}

	// 验证 span 属性
	attrs := span.Attributes
	hasMethod := false
	hasPath := false
	hasRoute := false
	hasStatusCode := false

	for _, attr := range attrs {
		switch attr.Key {
		case "http.request.method":
			hasMethod = true
			if attr.Value.AsString() != "GET" {
				t.Errorf("expected method GET, got %s", attr.Value.AsString())
			}
		case "url.path":
			hasPath = true
			if attr.Value.AsString() != "/users/123" {
				t.Errorf("expected path /users/123, got %s", attr.Value.AsString())
			}
		case "http.route":
			hasRoute = true
			if attr.Value.AsString() != "/users/:id" {
				t.Errorf("expected route /users/:id, got %s", attr.Value.AsString())
			}
		case "http.response.status_code":
			hasStatusCode = true
			if attr.Value.AsInt64() != 200 {
				t.Errorf("expected status code 200, got %d", attr.Value.AsInt64())
			}
		}
	}

	if !hasMethod {
		t.Error("span missing http.request.method attribute")
	}
	if !hasPath {
		t.Error("span missing url.path attribute")
	}
	if !hasRoute {
		t.Error("span missing http.route attribute")
	}
	if !hasStatusCode {
		t.Error("span missing http.response.status_code attribute")
	}
}

func TestTracingWithUpstreamContext(t *testing.T) {
	// 创建内存 exporter
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	defer provider.Shutdown(context.Background())

	// 设置全局 propagator
	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(propagator)

	// 创建上游 trace context
	upstreamTracer := provider.Tracer("upstream")
	upstreamCtx, upstreamSpan := upstreamTracer.Start(context.Background(), "upstream-operation")
	upstreamTraceID := upstreamSpan.SpanContext().TraceID().String()
	upstreamSpan.End()

	// 创建测试服务器
	engine, err := server.New(&server.Config{
		Mode: server.ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	engine.Use(Tracing(provider))
	engine.GET("/test", func(c *server.Context) {
		traceID := c.TraceID()
		if traceID != upstreamTraceID {
			t.Errorf("expected trace_id %s, got %s", upstreamTraceID, traceID)
		}
		c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// 创建带有 trace context 的请求
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	propagator.Inject(upstreamCtx, propagation.HeaderCarrier(req.Header))

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	// 验证响应
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// 验证 span 继承了上游的 trace_id
	spans := exporter.GetSpans()
	if len(spans) != 2 { // upstream span + server span
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	serverSpan := spans[1]
	if serverSpan.SpanContext.TraceID().String() != upstreamTraceID {
		t.Errorf("server span should inherit upstream trace_id")
	}
}

func TestTracingWithDefaultPropagator(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	defer provider.Shutdown(context.Background())

	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	defer otel.SetTextMapPropagator(previousPropagator)

	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)

	upstreamTracer := provider.Tracer("upstream")
	upstreamCtx, upstreamSpan := upstreamTracer.Start(context.Background(), "upstream-operation")
	upstreamTraceID := upstreamSpan.SpanContext().TraceID().String()
	upstreamSpan.End()

	engine, err := server.New(&server.Config{
		Mode: server.ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	engine.Use(Tracing(provider))
	engine.GET("/test", func(c *server.Context) {
		if traceID := c.TraceID(); traceID != upstreamTraceID {
			t.Errorf("expected trace_id %s, got %s", upstreamTraceID, traceID)
		}
		c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	propagator.Inject(upstreamCtx, propagation.HeaderCarrier(req.Header))

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	spans := exporter.GetSpans()
	if len(spans) < 2 {
		t.Fatalf("expected at least 2 spans, got %d", len(spans))
	}
	serverSpan := spans[len(spans)-1]
	if serverSpan.SpanContext.TraceID().String() != upstreamTraceID {
		t.Errorf("server span should inherit upstream trace_id")
	}
}

func TestTracingWithError(t *testing.T) {
	// 创建内存 exporter
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	defer provider.Shutdown(context.Background())

	// 创建测试服务器
	engine, err := server.New(&server.Config{
		Mode: server.ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	engine.Use(Tracing(provider))
	engine.GET("/error", func(c *server.Context) {
		c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	})

	// 发送测试请求
	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	// 验证响应
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}

	// 验证 span 状态为 Error
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	if span.Status.Code != codes.Error {
		t.Errorf("expected span status Error (code 1), got code %d (%s)", span.Status.Code, span.Status.Description)
	}
	if span.Status.Description != "HTTP 500" {
		t.Errorf("expected status description 'HTTP 500', got '%s'", span.Status.Description)
	}
}

func TestTracingWithClientErrorLeavesStatusUnset(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	defer provider.Shutdown(context.Background())

	engine, err := server.New(&server.Config{
		Mode: server.ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	engine.Use(Tracing(provider))
	engine.GET("/missing", func(c *server.Context) {
		c.AbortWithStatusJSON(http.StatusNotFound, map[string]string{"error": "not found"})
	})

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	if span.Status.Code != codes.Unset {
		t.Errorf("expected span status Unset, got code %d (%s)", span.Status.Code, span.Status.Description)
	}
}

func TestTracingWithPanicRecordsServerError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	defer provider.Shutdown(context.Background())

	engine, err := server.New(&server.Config{
		Mode: server.ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	engine.Use(Tracing(provider))
	engine.GET("/panic", func(c *server.Context) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	if span.Status.Code != codes.Error {
		t.Errorf("expected span status Error, got code %d (%s)", span.Status.Code, span.Status.Description)
	}
	if span.Status.Description != "panic recovered" {
		t.Errorf("expected status description 'panic recovered', got %q", span.Status.Description)
	}

	hasStatusCode := false
	for _, attr := range span.Attributes {
		if attr.Key == "http.response.status_code" {
			hasStatusCode = true
			if attr.Value.AsInt64() != http.StatusInternalServerError {
				t.Errorf("expected status code 500, got %d", attr.Value.AsInt64())
			}
		}
	}
	if !hasStatusCode {
		t.Error("span missing http.response.status_code attribute")
	}
}

func TestTracingRedactsSensitiveQuery(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	defer provider.Shutdown(context.Background())

	engine, err := server.New(&server.Config{
		Mode: server.ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	engine.Use(Tracing(provider))
	engine.GET("/search", func(c *server.Context) {
		c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/search?q=fox&token=secret-token&password=secret-password", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	for _, attr := range spans[0].Attributes {
		if attr.Key == "url.query" {
			query := attr.Value.AsString()
			if strings.Contains(query, "secret-token") || strings.Contains(query, "secret-password") {
				t.Fatalf("query contains sensitive value: %s", query)
			}
			if !strings.Contains(query, "q=fox") {
				t.Fatalf("query should keep non-sensitive values, got %s", query)
			}
			return
		}
	}
	t.Fatal("span missing url.query attribute")
}

func TestTracingWithNilProvider(t *testing.T) {
	// 测试 nil provider 时使用全局 provider
	engine, err := server.New(&server.Config{
		Mode: server.ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	engine.Use(Tracing(nil))
	engine.GET("/test", func(c *server.Context) {
		c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestTracingIntegrationWithTracingPackage(t *testing.T) {
	// 使用 tracing 包创建 provider
	cfg := &tracing.Config{
		Service: &tracing.ServiceConfig{
			Name:        "test-service",
			Version:     "1.0.0",
			Environment: "test",
		},
		Exporter: tracing.ExporterNone,
	}

	provider, err := tracing.New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("failed to create tracer provider: %v", err)
	}
	defer provider.Shutdown(context.Background())

	// 创建测试服务器
	engine, err := server.New(&server.Config{
		Mode: server.ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	engine.GET("/integration", func(c *server.Context) {
		traceID := c.TraceID()
		if traceID == "" {
			t.Error("trace_id not set")
		}
		c.JSON(http.StatusOK, map[string]string{"trace_id": traceID})
	})

	req := httptest.NewRequest(http.MethodGet, "/integration", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestTracingSpanContext(t *testing.T) {
	// 创建内存 exporter
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	defer provider.Shutdown(context.Background())

	var capturedSpanContext trace.SpanContext

	// 创建测试服务器
	engine, err := server.New(&server.Config{
		Mode: server.ModeTest,
		Addr: ":8080",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	engine.Use(Tracing(provider))
	engine.GET("/test", func(c *server.Context) {
		// 从标准 context 中获取 span context
		span := trace.SpanFromContext(c.StdContext())
		capturedSpanContext = span.SpanContext()
		c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// 验证 span context 有效
	if !capturedSpanContext.IsValid() {
		t.Error("span context should be valid")
	}

	if !capturedSpanContext.HasTraceID() {
		t.Error("span context should have trace ID")
	}

	if !capturedSpanContext.HasSpanID() {
		t.Error("span context should have span ID")
	}
}
