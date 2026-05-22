package tracing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var globalTracerProvider atomic.Pointer[sdktrace.TracerProvider]

// Provider 是 tracing 包创建并托管生命周期的 TracerProvider。
//
// 它嵌入 OpenTelemetry SDK TracerProvider，因此仍然可以直接调用 Tracer、
// ForceFlush 等原生方法；Shutdown 会额外清理 tracing 包维护的全局状态。
type Provider struct {
	*sdktrace.TracerProvider

	shutdownOnce sync.Once
	shutdownErr  error
}

// Shutdown 关闭 TracerProvider，并在当前全局 provider 仍然是自己时自动清理全局状态。
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil || p.TracerProvider == nil {
		return nil
	}

	p.shutdownOnce.Do(func() {
		if globalTracerProvider.CompareAndSwap(p.TracerProvider, nil) {
			otel.SetTracerProvider(oteltrace.NewNoopTracerProvider())
		}
		p.shutdownErr = p.TracerProvider.Shutdown(ctx)
	})
	return p.shutdownErr
}

// New 根据 Config 创建 OpenTelemetry TracerProvider。
//
// 函数会先执行 Config.Validate，然后按配置创建 resource、sampler、exporter 和
// span processor。New 会把创建出的 provider 记录为框架级全局 provider，并同步
// 设置为 OpenTelemetry 全局 provider，方便其他 instrumentation 复用。
func New(ctx context.Context, cfg *Config) (*Provider, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg == nil {
		return nil, errors.New("tracing config is nil")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	res, err := buildResource(cfg)
	if err != nil {
		return nil, err
	}

	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
	}
	if cfg.Sampler != nil {
		options = append(options, sdktrace.WithSampler(buildSampler(cfg.Sampler)))
	}

	exporter, err := buildExporter(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if exporter != nil {
		options = append(options, sdktrace.WithBatcher(exporter, buildBatchOptions(cfg.Batch)...))
	}

	provider := sdktrace.NewTracerProvider(options...)
	if !globalTracerProvider.CompareAndSwap(nil, provider) {
		_ = provider.Shutdown(ctx)
		return nil, errors.New("tracing provider already initialized")
	}
	otel.SetTracerProvider(provider)

	return &Provider{TracerProvider: provider}, nil
}

// GlobalTracerProvider 返回框架级全局 tracer provider。
func GlobalTracerProvider() (*sdktrace.TracerProvider, bool) {
	provider := globalTracerProvider.Load()
	if provider == nil {
		return nil, false
	}
	return provider, true
}

// buildExporter 根据配置创建 span exporter。
//
// ExporterNone 不创建 exporter，TracerProvider 仍然可用，但不会导出 span；stdout
// 适合本地调试；OTLP gRPC/HTTP 适合生产环境接入 OpenTelemetry Collector。
func buildExporter(ctx context.Context, cfg *Config) (sdktrace.SpanExporter, error) {
	switch cfg.exporterOrDefault() {
	case ExporterNone:
		return nil, nil
	case ExporterStdout:
		exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("create stdout trace exporter: %w", err)
		}
		return exporter, nil
	case ExporterOTLPGRPC:
		exporter, err := otlptracegrpc.New(ctx, buildOTLPGRPCOptions(cfg.OTLP)...)
		if err != nil {
			return nil, fmt.Errorf("create otlp grpc trace exporter: %w", err)
		}
		return exporter, nil
	case ExporterOTLPHTTP:
		exporter, err := otlptracehttp.New(ctx, buildOTLPHTTPOptions(cfg.OTLP)...)
		if err != nil {
			return nil, fmt.Errorf("create otlp http trace exporter: %w", err)
		}
		return exporter, nil
	default:
		return nil, fmt.Errorf("unsupported tracing exporter %q", cfg.Exporter)
	}
}

// buildOTLPGRPCOptions 将 OTLPConfig 映射为 OTLP gRPC exporter 选项。
func buildOTLPGRPCOptions(cfg *OTLPConfig) []otlptracegrpc.Option {
	if cfg == nil {
		return nil
	}

	options := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(strings.TrimSpace(cfg.Endpoint)),
	}
	if cfg.Insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		options = append(options, otlptracegrpc.WithHeaders(cfg.Headers))
	}
	if cfg.Timeout > 0 {
		options = append(options, otlptracegrpc.WithTimeout(cfg.Timeout))
	}
	if cfg.Compression == CompressionGzip {
		options = append(options, otlptracegrpc.WithCompressor("gzip"))
	}
	return options
}

// buildOTLPHTTPOptions 将 OTLPConfig 映射为 OTLP HTTP exporter 选项。
func buildOTLPHTTPOptions(cfg *OTLPConfig) []otlptracehttp.Option {
	if cfg == nil {
		return nil
	}

	endpoint := strings.TrimSpace(cfg.Endpoint)
	options := make([]otlptracehttp.Option, 0, 5)
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		options = append(options, otlptracehttp.WithEndpointURL(endpoint))
	} else {
		options = append(options, otlptracehttp.WithEndpoint(endpoint))
	}
	if cfg.Insecure {
		options = append(options, otlptracehttp.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		options = append(options, otlptracehttp.WithHeaders(cfg.Headers))
	}
	if cfg.Timeout > 0 {
		options = append(options, otlptracehttp.WithTimeout(cfg.Timeout))
	}
	if cfg.Compression == CompressionGzip {
		options = append(options, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
	}
	return options
}

// buildBatchOptions 将 BatchConfig 映射为 BatchSpanProcessorOption。
func buildBatchOptions(cfg *BatchConfig) []sdktrace.BatchSpanProcessorOption {
	if cfg == nil {
		return nil
	}

	options := make([]sdktrace.BatchSpanProcessorOption, 0, 4)
	if cfg.MaxQueueSize > 0 {
		options = append(options, sdktrace.WithMaxQueueSize(cfg.MaxQueueSize))
	}
	if cfg.BatchTimeout > 0 {
		options = append(options, sdktrace.WithBatchTimeout(cfg.BatchTimeout))
	}
	if cfg.ExportTimeout > 0 {
		options = append(options, sdktrace.WithExportTimeout(cfg.ExportTimeout))
	}
	if cfg.MaxExportBatchSize > 0 {
		options = append(options, sdktrace.WithMaxExportBatchSize(cfg.MaxExportBatchSize))
	}
	return options
}

// buildSampler 将采样配置映射为 sdktrace.Sampler。
func buildSampler(cfg *SamplerConfig) sdktrace.Sampler {
	switch cfg.typeOrDefault() {
	case SamplerAlwaysOn:
		return sdktrace.AlwaysSample()
	case SamplerAlwaysOff:
		return sdktrace.NeverSample()
	case SamplerTraceIDRatio:
		return sdktrace.TraceIDRatioBased(cfg.Ratio)
	case SamplerParentBasedAlwaysOn:
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	case SamplerParentBasedTraceIDRatio:
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.Ratio))
	default:
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
}

// buildResource 构造 TracerProvider 使用的 resource。
//
// 默认会保留 OpenTelemetry SDK 自带的 resource，并用 ServiceConfig 和 ResourceConfig
// 中的字段覆盖或补充稳定标签。
func buildResource(cfg *Config) (*resource.Resource, error) {
	attrs := make([]attribute.KeyValue, 0, 8)
	if cfg.Service != nil {
		attrs = appendServiceAttributes(attrs, cfg.Service)
	}
	if cfg.Resource != nil {
		for key, value := range cfg.Resource.Attributes {
			attrs = append(attrs, attribute.String(strings.TrimSpace(key), value))
		}
	}
	if len(attrs) == 0 {
		return resource.Default(), nil
	}

	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(attrs...))
	if err != nil {
		return nil, fmt.Errorf("merge tracing resource: %w", err)
	}
	return res, nil
}

func appendServiceAttributes(attrs []attribute.KeyValue, cfg *ServiceConfig) []attribute.KeyValue {
	if name := strings.TrimSpace(cfg.Name); name != "" {
		attrs = append(attrs, semconv.ServiceName(name))
	}
	if namespace := strings.TrimSpace(cfg.Namespace); namespace != "" {
		attrs = append(attrs, semconv.ServiceNamespace(namespace))
	}
	if version := strings.TrimSpace(cfg.Version); version != "" {
		attrs = append(attrs, semconv.ServiceVersion(version))
	}
	if instanceID := strings.TrimSpace(cfg.InstanceID); instanceID != "" {
		attrs = append(attrs, semconv.ServiceInstanceID(instanceID))
	}
	if environment := strings.TrimSpace(cfg.Environment); environment != "" {
		attrs = append(attrs, semconv.DeploymentEnvironmentName(environment))
	}
	return attrs
}
