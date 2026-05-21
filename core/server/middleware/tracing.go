package middleware

import (
	"github.com/surge-go/fox/core/server"
	"go.opentelemetry.io/otel/trace"
)

// Tracing 为每个 HTTP 请求创建 OpenTelemetry server span。
//
// 该函数保留给需要手动指定 tracer provider 的场景；如果应用已经通过
// core/tracing.New 初始化全局 provider，server.New 会自动注册内置 tracing。
func Tracing(tracerProvider trace.TracerProvider) server.HandlerFunc {
	return server.TracingMiddleware(tracerProvider)
}
