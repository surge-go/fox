package metrics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

var globalMeterProvider atomic.Pointer[sdkmetric.MeterProvider]

// Provider 是 metrics 包创建并托管生命周期的 MeterProvider。
type Provider struct {
	meterProvider      *sdkmetric.MeterProvider
	prometheusGatherer promclient.Gatherer

	shutdownOnce sync.Once
	shutdownErr  error
}

// MeterProvider 返回底层 SDK MeterProvider。
func (p *Provider) MeterProvider() *sdkmetric.MeterProvider {
	if p == nil {
		return nil
	}
	return p.meterProvider
}

// PrometheusGatherer 返回 Prometheus exporter 的抓取入口。
func (p *Provider) PrometheusGatherer() promclient.Gatherer {
	if p == nil {
		return nil
	}
	return p.prometheusGatherer
}

// Shutdown 关闭 MeterProvider，并在当前全局 provider 仍然是自己时自动清理全局状态。
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil || p.meterProvider == nil {
		return nil
	}

	p.shutdownOnce.Do(func() {
		if globalMeterProvider.CompareAndSwap(p.meterProvider, nil) {
			otel.SetMeterProvider(metricnoop.NewMeterProvider())
		}
		p.shutdownErr = p.meterProvider.Shutdown(ctx)
	})
	return p.shutdownErr
}

// New 根据 Config 创建 OpenTelemetry MeterProvider。
func New(ctx context.Context, cfg *Config) (*Provider, error) {
	return NewWithRegisterer(ctx, cfg, nil)
}

// NewWithRegisterer 根据 Config 和 Prometheus registerer 创建 MeterProvider。
//
// registerer 仅在 ExporterPrometheus 下使用；为 nil 时使用默认 registry。
func NewWithRegisterer(ctx context.Context, cfg *Config, registerer promclient.Registerer) (*Provider, error) {
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

	reader, gatherer, err := buildReader(ctx, cfg, registerer)
	if err != nil {
		return nil, err
	}
	if reader != nil {
		options = append(options, sdkmetric.WithReader(reader))
	}

	mp := sdkmetric.NewMeterProvider(options...)
	if !globalMeterProvider.CompareAndSwap(nil, mp) {
		_ = mp.Shutdown(ctx)
		return nil, errors.New("metrics provider already initialized")
	}
	otel.SetMeterProvider(mp)

	return &Provider{
		meterProvider:      mp,
		prometheusGatherer: gatherer,
	}, nil
}

func buildReader(ctx context.Context, cfg *Config, registerer promclient.Registerer) (sdkmetric.Reader, promclient.Gatherer, error) {
	switch cfg.exporterOrDefault() {
	case ExporterNone:
		return nil, nil, nil
	case ExporterPrometheus:
		var (
			gatherer promclient.Gatherer
			reg      promclient.Registerer = registerer
		)
		if registerer == nil {
			registry := promclient.NewRegistry()
			registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
			registry.MustRegister(collectors.NewGoCollector())
			reg = registry
			gatherer = registry
		} else if g, ok := registerer.(promclient.Gatherer); ok {
			gatherer = g
		}
		exporter, err := promexporter.New(buildPrometheusOptions(cfg.Prometheus, reg)...)
		if err != nil {
			return nil, nil, fmt.Errorf("create prometheus metrics exporter: %w", err)
		}
		return exporter, gatherer, nil
	case ExporterStdout:
		exporter, err := stdoutmetric.New(stdoutmetric.WithPrettyPrint())
		if err != nil {
			return nil, nil, fmt.Errorf("create stdout metrics exporter: %w", err)
		}
		return sdkmetric.NewPeriodicReader(exporter, buildPeriodicReaderOptions(cfg.Reader)...), nil, nil
	case ExporterOTLPGRPC:
		exporter, err := otlpmetricgrpc.New(ctx, buildOTLPGRPCOptions(cfg.OTLP)...)
		if err != nil {
			return nil, nil, fmt.Errorf("create otlp grpc metrics exporter: %w", err)
		}
		return sdkmetric.NewPeriodicReader(exporter, buildPeriodicReaderOptions(cfg.Reader)...), nil, nil
	case ExporterOTLPHTTP:
		exporter, err := otlpmetrichttp.New(ctx, buildOTLPHTTPOptions(cfg.OTLP)...)
		if err != nil {
			return nil, nil, fmt.Errorf("create otlp http metrics exporter: %w", err)
		}
		return sdkmetric.NewPeriodicReader(exporter, buildPeriodicReaderOptions(cfg.Reader)...), nil, nil
	default:
		return nil, nil, fmt.Errorf("unsupported metrics exporter %q", cfg.Exporter)
	}
}

func buildPrometheusOptions(cfg *PrometheusConfig, registerer promclient.Registerer) []promexporter.Option {
	options := make([]promexporter.Option, 0, 5)
	if registerer != nil {
		options = append(options, promexporter.WithRegisterer(registerer))
	}
	if cfg == nil {
		return options
	}
	if ns := strings.TrimSpace(cfg.Namespace); ns != "" {
		options = append(options, promexporter.WithNamespace(ns))
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
	if timeout := cfg.timeoutOrDefault(); timeout > 0 {
		options = append(options, otlpmetricgrpc.WithTimeout(timeout))
	}
	switch cfg.compressionOrDefault() {
	case CompressionGzip:
		options = append(options, otlpmetricgrpc.WithCompressor("gzip"))
	case CompressionNone:
	}
	return options
}

func buildOTLPHTTPOptions(cfg *OTLPConfig) []otlpmetrichttp.Option {
	if cfg == nil {
		return nil
	}

	endpoint := strings.TrimSpace(cfg.Endpoint)
	options := make([]otlpmetrichttp.Option, 0, 6)
	options = append(options, otlpmetrichttp.WithEndpointURL(endpoint))
	if urlPath := strings.TrimSpace(cfg.URLPath); urlPath != "" {
		options = append(options, otlpmetrichttp.WithURLPath(urlPath))
	}
	if cfg.Insecure {
		options = append(options, otlpmetrichttp.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		options = append(options, otlpmetrichttp.WithHeaders(cfg.Headers))
	}
	if timeout := cfg.timeoutOrDefault(); timeout > 0 {
		options = append(options, otlpmetrichttp.WithTimeout(timeout))
	}
	switch cfg.compressionOrDefault() {
	case CompressionGzip:
		options = append(options, otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression))
	case CompressionNone:
	}
	return options
}

func buildPeriodicReaderOptions(cfg *ReaderConfig) []sdkmetric.PeriodicReaderOption {
	if cfg == nil {
		cfg = &ReaderConfig{}
	}

	options := make([]sdkmetric.PeriodicReaderOption, 0, 2)
	if interval := cfg.intervalOrDefault(); interval > 0 {
		options = append(options, sdkmetric.WithInterval(interval))
	}
	if timeout := cfg.timeoutOrDefault(); timeout > 0 {
		options = append(options, sdkmetric.WithTimeout(timeout))
	}
	return options
}

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
