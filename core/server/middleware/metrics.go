package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/surge-go/fox/core/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/semconv/v1.37.0"
)

const defaultMetricsMeterName = "github.com/surge-go/fox/core/server/middleware"

// MetricsConfig 配置 HTTP server metrics 中间件。
type MetricsConfig struct {
	// MeterName 是 instrumentation scope name。为空时使用默认值。
	MeterName string

	// SkipFunc 返回 true 时跳过 metrics。
	SkipFunc func(c *server.Context) bool

	// AttributesFunc 追加自定义指标属性。
	//
	// 注意：指标属性会成为时间序列标签，应避免 path、query、用户 ID、IP、header
	// 等高基数或敏感字段。
	AttributesFunc func(c *server.Context) []attribute.KeyValue
}

// Metrics 返回 HTTP server 指标中间件。
//
// 默认使用 OpenTelemetry 全局 MeterProvider。configs 为可选配置，只读取第一个
// 配置；不传时使用默认配置。该中间件只记录低基数字段：method、status_code
// 和 route。
func Metrics(configs ...MetricsConfig) server.HandlerFunc {
	cfg := MetricsConfig{}
	if len(configs) > 0 {
		cfg = configs[0]
	}
	if cfg.MeterName == "" {
		cfg.MeterName = defaultMetricsMeterName
	}

	meter := otel.Meter(cfg.MeterName)
	requestCount, err := meter.Int64Counter(
		"http.server.request.count",
		metric.WithDescription("Total number of HTTP server requests."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		otel.Handle(err)
		return passThroughMiddleware()
	}
	requestDuration, err := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithDescription("Duration of HTTP server requests."),
		metric.WithUnit("s"),
	)
	if err != nil {
		otel.Handle(err)
		return passThroughMiddleware()
	}

	return func(c *server.Context) {
		if cfg.SkipFunc != nil && cfg.SkipFunc(c) {
			c.Next()
			return
		}

		start := time.Now()
		defer recordHTTPMetrics(c, cfg, requestCount, requestDuration, start)

		c.Next()
	}
}

func passThroughMiddleware() server.HandlerFunc {
	return func(c *server.Context) {
		c.Next()
	}
}

func recordHTTPMetrics(
	c *server.Context,
	cfg MetricsConfig,
	requestCount metric.Int64Counter,
	requestDuration metric.Float64Histogram,
	start time.Time,
) {
	statusCode := c.Status()
	if recovered := recover(); recovered != nil {
		statusCode = http.StatusInternalServerError
		recordHTTPMetricsData(c, cfg, requestCount, requestDuration, start, statusCode)
		panic(recovered)
	}

	recordHTTPMetricsData(c, cfg, requestCount, requestDuration, start, statusCode)
}

func recordHTTPMetricsData(
	c *server.Context,
	cfg MetricsConfig,
	requestCount metric.Int64Counter,
	requestDuration metric.Float64Histogram,
	start time.Time,
	statusCode int,
) {
	attrs := metricAttributes(c, cfg, statusCode)
	opts := metric.WithAttributes(attrs...)
	ctx := c.StdContext()

	requestCount.Add(ctx, 1, opts)
	requestDuration.Record(ctx, time.Since(start).Seconds(), opts)
}

func metricAttributes(c *server.Context, cfg MetricsConfig, statusCode int) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(c.RawRequest().Method),
		semconv.HTTPResponseStatusCode(statusCode),
	}
	if route := c.FullPath(); route != "" {
		attrs = append(attrs, semconv.HTTPRoute(route))
	}
	if cfg.AttributesFunc != nil {
		attrs = append(attrs, safeCustomMetricAttributes(c, cfg.AttributesFunc)...)
	}
	return attrs
}

func safeCustomMetricAttributes(
	c *server.Context,
	fn func(c *server.Context) []attribute.KeyValue,
) (attrs []attribute.KeyValue) {
	defer func() {
		if recovered := recover(); recovered != nil {
			otel.Handle(fmt.Errorf("metrics attributes function panic: %v", recovered))
			attrs = nil
		}
	}()

	for _, attr := range fn(c) {
		if isReservedHTTPMetricAttribute(attr.Key) {
			continue
		}
		attrs = append(attrs, attr)
	}
	return attrs
}

func isReservedHTTPMetricAttribute(key attribute.Key) bool {
	switch key {
	case semconv.HTTPRequestMethodKey,
		semconv.HTTPResponseStatusCodeKey,
		semconv.HTTPRouteKey:
		return true
	default:
		return false
	}
}
