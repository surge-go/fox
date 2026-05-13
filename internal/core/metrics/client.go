package metrics

import (
	"context"
	"errors"
	"fmt"
	"strings"

	promclient "github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

// New 根据 Config 创建 OpenTelemetry MeterProvider。
//
// 函数会先执行 Config.Validate，然后按配置创建 resource、reader 和 exporter。
// New 不会调用 otel.SetMeterProvider，也不会替换全局 provider；如果应用需要全局
// MeterProvider，应在启动层显式调用 otel.SetMeterProvider，并在退出时调用
// provider.Shutdown(ctx)。
//
// Prometheus exporter 默认注册到 prometheus.DefaultRegisterer。生产服务通常建议
// 使用 NewWithRegisterer 传入业务自己的 registry，再由启动层通过 promhttp.HandlerFor
// 把该 registry 挂到 /metrics。
func New(ctx context.Context, cfg *Config) (*sdkmetric.MeterProvider, error) {
	return NewWithRegisterer(ctx, cfg, nil)
}

// NewWithRegisterer 根据 Config 和 Prometheus registerer 创建 OpenTelemetry MeterProvider。
//
// registerer 仅在 ExporterPrometheus 下使用；为 nil 时使用 Prometheus exporter 默认行为。
// 该入口不会注册 HTTP 路由，调用方应在启动层自行决定 /metrics 路径、鉴权和监听端口。
func NewWithRegisterer(ctx context.Context, cfg *Config, registerer promclient.Registerer) (*sdkmetric.MeterProvider, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg == nil {
		return nil, errors.New("metrics config is nil")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	res, err := buildResource(cfg)
	if err != nil {
		return nil, err
	}

	options := []sdkmetric.Option{
		sdkmetric.WithResource(res),
	}

	reader, err := buildReader(ctx, cfg, registerer)
	if err != nil {
		return nil, err
	}
	if reader != nil {
		options = append(options, sdkmetric.WithReader(reader))
	}

	return sdkmetric.NewMeterProvider(options...), nil
}

// buildReader 根据 exporter 类型创建 metrics reader。
//
// Prometheus 使用 pull 模式，直接把 prometheus.Exporter 作为 reader；stdout 和 OTLP
// 使用 PeriodicReader 周期性导出；ExporterNone 不创建 reader。
func buildReader(ctx context.Context, cfg *Config, registerer promclient.Registerer) (sdkmetric.Reader, error) {
	switch cfg.exporterOrDefault() {
	case ExporterNone:
		return nil, nil
	case ExporterPrometheus:
		exporter, err := promexporter.New(buildPrometheusOptions(cfg.Prometheus, registerer)...)
		if err != nil {
			return nil, fmt.Errorf("create prometheus metrics exporter: %w", err)
		}
		return exporter, nil
	case ExporterStdout:
		exporter, err := stdoutmetric.New(stdoutmetric.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("create stdout metrics exporter: %w", err)
		}
		return sdkmetric.NewPeriodicReader(exporter, buildPeriodicReaderOptions(cfg.Reader)...), nil
	case ExporterOTLPGRPC:
		exporter, err := otlpmetricgrpc.New(ctx, buildOTLPGRPCOptions(cfg.OTLP)...)
		if err != nil {
			return nil, fmt.Errorf("create otlp grpc metrics exporter: %w", err)
		}
		return sdkmetric.NewPeriodicReader(exporter, buildPeriodicReaderOptions(cfg.Reader)...), nil
	case ExporterOTLPHTTP:
		exporter, err := otlpmetrichttp.New(ctx, buildOTLPHTTPOptions(cfg.OTLP)...)
		if err != nil {
			return nil, fmt.Errorf("create otlp http metrics exporter: %w", err)
		}
		return sdkmetric.NewPeriodicReader(exporter, buildPeriodicReaderOptions(cfg.Reader)...), nil
	default:
		return nil, fmt.Errorf("unsupported metrics exporter %q", cfg.Exporter)
	}
}

// buildPrometheusOptions 将 PrometheusConfig 映射为 prometheus exporter 选项。
func buildPrometheusOptions(cfg *PrometheusConfig, registerer promclient.Registerer) []promexporter.Option {
	options := make([]promexporter.Option, 0, 5)
	if registerer != nil {
		options = append(options, promexporter.WithRegisterer(registerer))
	}
	if cfg == nil {
		return options
	}
	if cfg.Namespace != "" {
		options = append(options, promexporter.WithNamespace(strings.TrimSpace(cfg.Namespace)))
	}
	if cfg.WithoutTargetInfo {
		options = append(options, promexporter.WithoutTargetInfo())
	}
	if cfg.WithoutScopeInfo {
		options = append(options, promexporter.WithoutScopeInfo())
	}
	if len(cfg.ResourceAttributesAsConstantLabels) > 0 {
		keys := make([]attribute.Key, 0, len(cfg.ResourceAttributesAsConstantLabels))
		for _, key := range cfg.ResourceAttributesAsConstantLabels {
			keys = append(keys, attribute.Key(strings.TrimSpace(key)))
		}
		options = append(options, promexporter.WithResourceAsConstantLabels(attribute.NewAllowKeysFilter(keys...)))
	}
	return options
}

// buildOTLPGRPCOptions 将 OTLPConfig 映射为 OTLP gRPC exporter 选项。
func buildOTLPGRPCOptions(cfg *OTLPConfig) []otlpmetricgrpc.Option {
	if cfg == nil {
		return nil
	}

	options := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(strings.TrimSpace(cfg.Endpoint)),
	}
	if cfg.Insecure {
		options = append(options, otlpmetricgrpc.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		options = append(options, otlpmetricgrpc.WithHeaders(cfg.Headers))
	}
	if cfg.Timeout > 0 {
		options = append(options, otlpmetricgrpc.WithTimeout(cfg.Timeout))
	}
	if cfg.Compression == CompressionGzip {
		options = append(options, otlpmetricgrpc.WithCompressor("gzip"))
	}
	return options
}

// buildOTLPHTTPOptions 将 OTLPConfig 映射为 OTLP HTTP exporter 选项。
func buildOTLPHTTPOptions(cfg *OTLPConfig) []otlpmetrichttp.Option {
	if cfg == nil {
		return nil
	}

	endpoint := strings.TrimSpace(cfg.Endpoint)
	options := make([]otlpmetrichttp.Option, 0, 6)
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		options = append(options, otlpmetrichttp.WithEndpointURL(endpoint))
	} else {
		options = append(options, otlpmetrichttp.WithEndpoint(endpoint))
	}
	if urlPath := strings.TrimSpace(cfg.URLPath); urlPath != "" {
		options = append(options, otlpmetrichttp.WithURLPath(urlPath))
	}
	if cfg.Insecure {
		options = append(options, otlpmetrichttp.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		options = append(options, otlpmetrichttp.WithHeaders(cfg.Headers))
	}
	if cfg.Timeout > 0 {
		options = append(options, otlpmetrichttp.WithTimeout(cfg.Timeout))
	}
	if cfg.Compression == CompressionGzip {
		options = append(options, otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression))
	}
	return options
}

// buildPeriodicReaderOptions 将 ReaderConfig 映射为 PeriodicReaderOption。
func buildPeriodicReaderOptions(cfg *ReaderConfig) []sdkmetric.PeriodicReaderOption {
	if cfg == nil {
		return nil
	}

	options := make([]sdkmetric.PeriodicReaderOption, 0, 2)
	if cfg.Interval > 0 {
		options = append(options, sdkmetric.WithInterval(cfg.Interval))
	}
	if cfg.Timeout > 0 {
		options = append(options, sdkmetric.WithTimeout(cfg.Timeout))
	}
	return options
}

// buildResource 构造 MeterProvider 使用的 resource。
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
		return nil, fmt.Errorf("merge metrics resource: %w", err)
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
