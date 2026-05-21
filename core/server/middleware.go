package server

import (
	"fmt"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "fox.server.middleware"

var sensitiveQueryKeys = map[string]struct{}{
	"api_key":       {},
	"apikey":        {},
	"auth":          {},
	"authorization": {},
	"client_secret": {},
	"code":          {},
	"id_token":      {},
	"password":      {},
	"passwd":        {},
	"pwd":           {},
	"refresh_token": {},
	"secret":        {},
	"sig":           {},
	"signature":     {},
	"token":         {},
}

// recoveryMiddleware 内置的 Recovery 中间件。
func recoveryMiddleware(mode Mode) HandlerFunc {
	return func(c *Context) {
		defer func() {
			if err := recover(); err != nil {
				if mode == ModeRelease {
					fmt.Println("[Recovery] panic recovered")
				} else {
					stack := debug.Stack()
					fmt.Printf("[Recovery] panic recovered:\n%v\n%s\n", err, stack)
				}

				c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
					"code":    500,
					"message": "Internal Server Error",
				})
			}
		}()

		c.Next()
	}
}

// loggerMiddleware 内置日志中间件，记录每个请求的基本信息。
func loggerMiddleware() HandlerFunc {
	return func(c *Context) {
		start := time.Now()

		defer func() {
			statusCode := c.Status()
			if recovered := recover(); recovered != nil {
				statusCode = http.StatusInternalServerError
				logRequest(c, start, statusCode)
				panic(recovered)
			}
			logRequest(c, start, statusCode)
		}()

		c.Next()
	}
}

func logRequest(c *Context, start time.Time, statusCode int) {
	latency := time.Since(start)
	clientIP := c.ClientIP()
	method := c.RawRequest().Method
	path := c.RawRequest().URL.Path

	traceID := c.GetString("trace_id")
	traceIDStr := ""
	if traceID != "" {
		traceIDStr = " " + traceID
	}

	fmt.Printf("[FOX] %s | %3d | %10s | %15s | %-7s \"%s\"%s\n",
		start.Format("2006/01/02 - 15:04:05"),
		statusCode,
		formatLogLatency(latency),
		clientIP,
		method,
		path,
		traceIDStr,
	)
}

// TracingMiddleware 为每个 HTTP 请求创建 OpenTelemetry server span。
//
// 中间件会提取入站 trace context，启动 server span，将活跃 span 写入请求
// context，并通过 server.Context 暴露 trace ID 供日志关联。provider 为 nil 时
// 使用全局 OpenTelemetry provider。
func TracingMiddleware(tracerProvider trace.TracerProvider) HandlerFunc {
	if tracerProvider == nil {
		tracerProvider = otel.GetTracerProvider()
	}

	tracer := tracerProvider.Tracer(tracerName)
	propagator := textMapPropagatorOrDefault(otel.GetTextMapPropagator())

	return func(c *Context) {
		req := c.RawRequest()
		ctx := req.Context()

		ctx = propagator.Extract(ctx, propagation.HeaderCarrier(req.Header))

		spanName := req.Method
		if route := c.FullPath(); route != "" {
			spanName = req.Method + " " + route
		}

		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(httpServerAttributes(c)...),
		)
		defer span.End()

		*req = *req.WithContext(ctx)

		if span.SpanContext().HasTraceID() {
			traceID := span.SpanContext().TraceID().String()
			c.Set("trace_id", traceID)
		}

		defer func() {
			if err := recover(); err != nil {
				span.RecordError(fmt.Errorf("panic: %v", err))
				setHTTPSpanResponse(span, http.StatusInternalServerError)
				span.SetStatus(codes.Error, "panic recovered")
				panic(err)
			}
		}()

		c.Next()

		statusCode := c.Status()
		setHTTPSpanResponse(span, statusCode)
	}
}

func textMapPropagatorOrDefault(propagator propagation.TextMapPropagator) propagation.TextMapPropagator {
	if propagator != nil && len(propagator.Fields()) > 0 {
		return propagator
	}
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func setHTTPSpanResponse(span trace.Span, statusCode int) {
	span.SetAttributes(semconv.HTTPResponseStatusCode(statusCode))
	switch {
	case statusCode < 100 || statusCode > 599:
		span.SetStatus(codes.Error, fmt.Sprintf("Invalid HTTP status code %d", statusCode))
	case statusCode >= http.StatusInternalServerError:
		span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", statusCode))
	}
}

func httpServerAttributes(c *Context) []attribute.KeyValue {
	req := c.RawRequest()

	attrs := []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(req.Method),
		semconv.URLPath(req.URL.Path),
		semconv.URLScheme(req.URL.Scheme),
		semconv.NetworkProtocolVersion(req.Proto),
		semconv.UserAgentOriginal(req.UserAgent()),
	}

	if query := redactURLQuery(req.URL.RawQuery); query != "" {
		attrs = append(attrs, semconv.URLQuery(query))
	}

	if clientIP := c.ClientIP(); clientIP != "" {
		attrs = append(attrs, semconv.ClientAddress(clientIP))
	}

	if host := req.Host; host != "" {
		attrs = append(attrs, semconv.ServerAddress(host))
	}

	if route := c.FullPath(); route != "" {
		attrs = append(attrs, semconv.HTTPRoute(route))
	}

	return attrs
}

func redactURLQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}

	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "[redacted]"
	}

	for key := range values {
		if _, ok := sensitiveQueryKeys[strings.ToLower(key)]; ok {
			values[key] = []string{"[REDACTED]"}
		}
	}
	return values.Encode()
}

func formatLogLatency(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%7dns", d.Nanoseconds())
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%7dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%7dms", d.Milliseconds())
	}
	return fmt.Sprintf("%7.2fs", d.Seconds())
}
