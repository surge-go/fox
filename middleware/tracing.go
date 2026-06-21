package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/surge-go/fox"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultTracingTracerName          = "github.com/surge-go/fox/middleware"
	defaultTracingResponseTraceHeader = "X-Trace-ID"
)

// TracingSpanNameFunc 根据请求生成 span 名称。
type TracingSpanNameFunc func(*fox.Context) string

// TracingSkipFunc 判断当前请求是否跳过 tracing。
type TracingSkipFunc func(*fox.Context) bool

// TracingConfig 表示链路追踪中间件配置。
type TracingConfig struct {
	// TracerProvider 是 OpenTelemetry tracer provider，nil 时每次请求使用当前全局 provider。
	TracerProvider trace.TracerProvider
	// TracerName 是 instrumentation scope 名称，空值使用默认名称。
	TracerName string
	// Propagators 用于从请求头提取 trace context，nil 使用 W3C TraceContext 和 Baggage。
	Propagators propagation.TextMapPropagator
	// SpanNameFunc 根据请求生成 span 名称，nil 使用 DefaultTracingSpanName。
	SpanNameFunc TracingSpanNameFunc
	// SkipPaths 表示跳过 tracing 的请求路径。
	SkipPaths []string
	// SkipFunc 判断是否跳过 tracing。
	SkipFunc TracingSkipFunc
	// ResponseTraceHeader 表示写入 trace id 的响应头，空值使用默认响应头。
	ResponseTraceHeader string
	// DisableResponseTraceHeader 控制是否关闭 trace id 响应头。
	DisableResponseTraceHeader bool
	// RecordHeaders 表示记录到 span 属性中的请求头名称，敏感请求头会被自动忽略。
	RecordHeaders []string
}

// DefaultTracingConfig 返回链路追踪中间件默认配置。
func DefaultTracingConfig() TracingConfig {
	return TracingConfig{
		TracerName:          defaultTracingTracerName,
		Propagators:         propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
		SpanNameFunc:        DefaultTracingSpanName,
		ResponseTraceHeader: defaultTracingResponseTraceHeader,
	}
}

// Tracing 返回使用默认配置的链路追踪中间件。
func Tracing() fox.HandlerFunc {
	return TracingWithConfig(DefaultTracingConfig())
}

// TracingWithConfig 返回使用自定义配置的链路追踪中间件。
func TracingWithConfig(cfg TracingConfig) fox.HandlerFunc {
	cfg = normalizeTracingConfig(cfg)
	skipPaths := makeTracingSkipPaths(cfg.SkipPaths)
	recordHeaders := normalizeTracingHeaders(cfg.RecordHeaders)

	return func(c *fox.Context) {
		if shouldSkipTracing(c, skipPaths, cfg.SkipFunc) {
			c.Next()
			return
		}

		req := c.RawRequest()
		parent := cfg.Propagators.Extract(req.Context(), propagation.HeaderCarrier(req.Header))
		startName := req.Method + " " + req.URL.Path
		ctx, span := tracerFromTracingConfig(cfg).Start(parent, startName, trace.WithSpanKind(trace.SpanKindServer))
		defer func() {
			recovered := recover()
			status := tracingResponseStatus(c, recovered != nil)
			span.SetName(cfg.SpanNameFunc(c))
			span.SetAttributes(responseTracingAttributes(c, status)...)
			if recovered != nil {
				span.RecordError(tracingPanicError(recovered))
				span.SetStatus(codes.Error, fmt.Sprint(recovered))
				span.End()
				panic(recovered)
			}
			if status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(status))
			}
			span.End()
		}()

		c.WithContext(ctx)
		spanContext := span.SpanContext()
		if spanContext.HasTraceID() {
			traceID := spanContext.TraceID().String()
			c.SetTraceID(traceID)
			if !cfg.DisableResponseTraceHeader && cfg.ResponseTraceHeader != "" {
				c.SetHeader(cfg.ResponseTraceHeader, traceID)
			}
		}
		if spanContext.HasSpanID() {
			c.SetSpanID(spanContext.SpanID().String())
		}

		span.SetAttributes(requestTracingAttributes(c, recordHeaders)...)
		c.Next()
	}
}

// DefaultTracingSpanName 返回默认 HTTP server span 名称。
func DefaultTracingSpanName(c *fox.Context) string {
	req := c.RawRequest()
	if route := c.FullPath(); route != "" {
		return req.Method + " " + route
	}
	return req.Method + " " + req.URL.Path
}

func normalizeTracingConfig(cfg TracingConfig) TracingConfig {
	defaults := DefaultTracingConfig()
	if strings.TrimSpace(cfg.TracerName) == "" {
		cfg.TracerName = defaults.TracerName
	}
	if cfg.Propagators == nil {
		cfg.Propagators = defaults.Propagators
	}
	if cfg.SpanNameFunc == nil {
		cfg.SpanNameFunc = defaults.SpanNameFunc
	}
	if cfg.ResponseTraceHeader == "" {
		cfg.ResponseTraceHeader = defaults.ResponseTraceHeader
	}
	return cfg
}

func tracerFromTracingConfig(cfg TracingConfig) trace.Tracer {
	provider := cfg.TracerProvider
	if provider == nil {
		provider = otel.GetTracerProvider()
	}
	return provider.Tracer(cfg.TracerName)
}

func tracingResponseStatus(c *fox.Context, panicked bool) int {
	if panicked && !c.Written() {
		return http.StatusInternalServerError
	}
	if status := c.Status(); status > 0 {
		return status
	}
	return http.StatusOK
}

func tracingPanicError(recovered any) error {
	if err, ok := recovered.(error); ok {
		return err
	}
	return fmt.Errorf("%v", recovered)
}

func makeTracingSkipPaths(paths []string) map[string]struct{} {
	if len(paths) == 0 {
		return nil
	}
	skipPaths := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			skipPaths[path] = struct{}{}
		}
	}
	return skipPaths
}

func shouldSkipTracing(c *fox.Context, skipPaths map[string]struct{}, skipFunc TracingSkipFunc) bool {
	if skipFunc != nil && skipFunc(c) {
		return true
	}
	if len(skipPaths) == 0 {
		return false
	}
	_, ok := skipPaths[c.RawRequest().URL.Path]
	return ok
}

func requestTracingAttributes(c *fox.Context, recordHeaders []string) []attribute.KeyValue {
	req := c.RawRequest()
	attrs := []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(req.Method),
		semconv.URLPath(req.URL.Path),
		semconv.ClientAddress(c.ClientIP()),
	}
	if host := req.Host; host != "" {
		attrs = append(attrs, semconv.ServerAddress(hostWithoutPort(host)))
	}
	if req.Proto != "" {
		attrs = append(attrs, semconv.NetworkProtocolName("http"))
		if version := httpProtocolVersion(req.Proto); version != "" {
			attrs = append(attrs, semconv.NetworkProtocolVersion(version))
		}
	}
	if userAgent := req.UserAgent(); userAgent != "" {
		attrs = append(attrs, semconv.UserAgentOriginal(userAgent))
	}
	for _, header := range recordHeaders {
		values := req.Header.Values(header)
		if len(values) > 0 {
			attrs = append(attrs, tracingHeaderAttributeKey(header).StringSlice(values))
		}
	}
	return attrs
}

func responseTracingAttributes(c *fox.Context, status int) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.HTTPResponseStatusCode(status),
	}
	if route := c.FullPath(); route != "" {
		attrs = append(attrs, semconv.HTTPRoute(route))
	}
	return attrs
}

func normalizeTracingHeaders(headers []string) []string {
	if len(headers) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(headers))
	seen := make(map[string]struct{}, len(headers))
	for _, header := range headers {
		header = http.CanonicalHeaderKey(strings.TrimSpace(header))
		if header == "" {
			continue
		}
		lower := strings.ToLower(header)
		if _, ok := seen[lower]; ok {
			continue
		}
		if _, ok := sensitiveTracingHeaders[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		normalized = append(normalized, header)
	}
	return normalized
}

var sensitiveTracingHeaders = map[string]struct{}{
	"authorization":       {},
	"cookie":              {},
	"proxy-authorization": {},
	"set-cookie":          {},
	"x-api-key":           {},
}

func tracingHeaderAttributeKey(header string) attribute.Key {
	key := strings.ToLower(strings.TrimSpace(header))
	key = strings.ReplaceAll(key, "-", "_")
	return attribute.Key("http.request.header." + key)
}

func hostWithoutPort(host string) string {
	if strings.HasPrefix(host, "[") {
		if value, _, err := net.SplitHostPort(host); err == nil {
			return strings.Trim(value, "[]")
		}
		return strings.Trim(host, "[]")
	}
	value, _, err := net.SplitHostPort(host)
	if err == nil {
		return value
	}
	return host
}

func httpProtocolVersion(proto string) string {
	switch strings.ToUpper(strings.TrimSpace(proto)) {
	case "HTTP/1.0":
		return "1.0"
	case "HTTP/1.1":
		return "1.1"
	case "HTTP/2", "HTTP/2.0":
		return "2.0"
	case "HTTP/3", "HTTP/3.0":
		return "3.0"
	default:
		return ""
	}
}
