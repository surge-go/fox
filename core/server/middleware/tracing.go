package middleware

import (
	"net/http"
	"strings"

	"github.com/surge-go/fox/core/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

const defaultTracingTracerName = "github.com/surge-go/fox/core/server/middleware"

// TracingConfig 配置 HTTP server tracing 中间件。
type TracingConfig struct {
	// TracerName 是 instrumentation scope name。为空时使用默认值。
	TracerName string

	// SpanNameFunc 自定义 span 名称。nil 时使用 "METHOD route"。
	SpanNameFunc func(c *server.Context) string

	// SkipFunc 返回 true 时跳过 tracing。
	SkipFunc func(c *server.Context) bool

	// RecordRequestHeaders 是需要记录到 span attributes 的请求头白名单。
	RecordRequestHeaders []string

	// RecordResponseHeaders 是需要记录到 span attributes 的响应头白名单。
	RecordResponseHeaders []string

	// RecordQuery 控制是否记录 URL query。默认不记录，避免敏感信息和高基数。
	RecordQuery bool
}

// Tracing 返回 HTTP server 链路追踪中间件。
//
// 默认使用 OpenTelemetry 全局 TracerProvider 和 TextMapPropagator。configs 为可选
// 配置，只读取第一个配置；不传时使用默认配置。
func Tracing(configs ...TracingConfig) server.HandlerFunc {
	cfg := TracingConfig{}
	if len(configs) > 0 {
		cfg = configs[0]
	}
	if cfg.TracerName == "" {
		cfg.TracerName = defaultTracingTracerName
	}

	tracer := otel.Tracer(cfg.TracerName)

	return func(c *server.Context) {
		if cfg.SkipFunc != nil && cfg.SkipFunc(c) {
			c.Next()
			return
		}

		req := c.RawRequest()
		propagator := otel.GetTextMapPropagator()
		ctx := propagator.Extract(c.StdContext(), propagation.HeaderCarrier(req.Header))
		ctx, span := tracer.Start(
			ctx,
			requestSpanName(req.Method, req.URL.Path),
			trace.WithSpanKind(trace.SpanKindServer),
		)
		defer finishServerSpan(c, cfg, span)

		c.WithContext(ctx)
		span.SetAttributes(requestAttributes(c, cfg)...)

		c.Next()
	}
}

func finishServerSpan(c *server.Context, cfg TracingConfig, span trace.Span) {
	if recovered := recover(); recovered != nil {
		span.SetName(finalSpanName(c, cfg))
		span.SetAttributes(responseAttributes(c, cfg)...)
		span.SetAttributes(semconv.HTTPResponseStatusCode(http.StatusInternalServerError))
		span.SetStatus(codes.Error, "panic")
		span.End()
		panic(recovered)
	}

	span.SetName(finalSpanName(c, cfg))
	span.SetAttributes(responseAttributes(c, cfg)...)
	statusCode := c.Status()
	if statusCode >= http.StatusInternalServerError {
		span.SetStatus(codes.Error, http.StatusText(statusCode))
	}
	span.End()
}

func requestAttributes(c *server.Context, cfg TracingConfig) []attribute.KeyValue {
	req := c.RawRequest()
	attrs := []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(req.Method),
		semconv.URLPath(req.URL.Path),
		semconv.ClientAddress(c.ClientIP()),
	}

	if userAgent := c.GetHeader("User-Agent"); userAgent != "" {
		attrs = append(attrs, semconv.UserAgentOriginal(userAgent))
	}
	if cfg.RecordQuery && req.URL.RawQuery != "" {
		attrs = append(attrs, semconv.URLQuery(req.URL.RawQuery))
	}
	for _, header := range cfg.RecordRequestHeaders {
		if value := c.GetHeader(header); value != "" {
			attrs = append(attrs, semconv.HTTPRequestHeader(normalizeHeaderName(header), value))
		}
	}
	return attrs
}

func responseAttributes(c *server.Context, cfg TracingConfig) []attribute.KeyValue {
	statusCode := c.Status()
	attrs := []attribute.KeyValue{
		semconv.HTTPResponseStatusCode(statusCode),
	}

	if route := c.FullPath(); route != "" {
		attrs = append(attrs, semconv.HTTPRoute(route))
	}
	for _, header := range cfg.RecordResponseHeaders {
		if value := c.GetResponseHeader(header); value != "" {
			attrs = append(attrs, semconv.HTTPResponseHeader(normalizeHeaderName(header), value))
		}
	}
	return attrs
}

func finalSpanName(c *server.Context, cfg TracingConfig) string {
	if cfg.SpanNameFunc != nil {
		if name := strings.TrimSpace(cfg.SpanNameFunc(c)); name != "" {
			return name
		}
	}

	route := c.FullPath()
	if route == "" {
		route = c.RawRequest().URL.Path
	}
	return requestSpanName(c.RawRequest().Method, route)
}

func requestSpanName(method, target string) string {
	method = strings.TrimSpace(method)
	if method == "" {
		method = "HTTP"
	}
	target = strings.TrimSpace(target)
	if target == "" {
		target = "/"
	}
	return method + " " + target
}

func normalizeHeaderName(header string) string {
	return strings.ToLower(strings.TrimSpace(header))
}
