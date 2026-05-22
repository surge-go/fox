package metrics

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Exporter 表示 metrics 数据导出方式。
type Exporter string

const (
	// ExporterNone 表示不导出 metrics 数据，但仍创建并设置全局 MeterProvider。
	ExporterNone Exporter = "none"

	// ExporterStdout 表示把 metrics 数据输出到 stdout，通常用于本地调试。
	ExporterStdout Exporter = "stdout"

	// ExporterOTLPGRPC 表示通过 OTLP gRPC 协议导出 metrics 数据。
	ExporterOTLPGRPC Exporter = "otlp_grpc"

	// ExporterOTLPHTTP 表示通过 OTLP HTTP/protobuf 协议导出 metrics 数据。
	ExporterOTLPHTTP Exporter = "otlp_http"

	// ExporterPrometheus 表示通过 Prometheus pull 模式暴露 metrics 数据。
	ExporterPrometheus Exporter = "prometheus"
)

// Compression 表示 OTLP exporter 压缩方式。
type Compression string

const (
	// CompressionNone 表示不压缩。
	CompressionNone Compression = "none"

	// CompressionGzip 表示使用 gzip 压缩。
	CompressionGzip Compression = "gzip"
)

const (
	defaultReaderInterval = 15 * time.Second
	defaultReaderTimeout  = 5 * time.Second
	defaultOTLPTimeout    = 5 * time.Second
)

// Config 表示 OpenTelemetry metrics 配置。
type Config struct {
	// Service 是当前服务的资源信息。nil 表示使用 OpenTelemetry 默认 resource。
	Service *ServiceConfig `json:"service" yaml:"service" mapstructure:"service"`

	// Exporter 指定 metrics 数据导出方式。为空时使用 ExporterNone。
	Exporter Exporter `json:"exporter" yaml:"exporter" mapstructure:"exporter"`

	// Resource 是额外 resource attributes。
	Resource *ResourceConfig `json:"resource" yaml:"resource" mapstructure:"resource"`

	// Reader 是周期性导出 reader 配置，仅 stdout、otlp_grpc、otlp_http 使用。
	Reader *ReaderConfig `json:"reader" yaml:"reader" mapstructure:"reader"`

	// OTLP 是 OTLP exporter 配置，仅 otlp_grpc、otlp_http 使用。
	OTLP *OTLPConfig `json:"otlp" yaml:"otlp" mapstructure:"otlp"`

	// Prometheus 是 Prometheus exporter 配置，仅 prometheus 使用。
	Prometheus *PrometheusConfig `json:"prometheus" yaml:"prometheus" mapstructure:"prometheus"`
}

// Validate 校验 metrics 配置是否满足创建 MeterProvider 的基本要求。
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("metrics config is nil")
	}

	var errs []error
	exporter := c.exporterOrDefault()

	if !isValidExporter(exporter) {
		errs = append(errs, fmt.Errorf("metrics exporter must be one of %q, %q, %q, %q, %q", ExporterNone, ExporterStdout, ExporterOTLPGRPC, ExporterOTLPHTTP, ExporterPrometheus))
	}
	if requiresOTLP(exporter) && c.OTLP == nil {
		errs = append(errs, errors.New("metrics otlp config is required when exporter is otlp_grpc or otlp_http"))
	}
	if !requiresOTLP(exporter) && c.OTLP != nil {
		errs = append(errs, errors.New("metrics otlp config requires exporter to be otlp_grpc or otlp_http"))
	}
	if exporter != ExporterPrometheus && c.Prometheus != nil {
		errs = append(errs, errors.New("metrics prometheus config requires exporter to be prometheus"))
	}
	if !usesPeriodicReader(exporter) && c.Reader != nil {
		errs = append(errs, errors.New("metrics reader config requires exporter to be stdout, otlp_grpc, or otlp_http"))
	}
	if c.Service != nil {
		errs = append(errs, c.Service.validate()...)
	}
	if c.Resource != nil {
		errs = append(errs, c.Resource.validate()...)
	}
	if c.Reader != nil {
		errs = append(errs, c.Reader.validate()...)
	}
	if c.OTLP != nil {
		errs = append(errs, c.OTLP.validate(exporter)...)
	}
	if c.Prometheus != nil {
		errs = append(errs, c.Prometheus.validate()...)
	}

	return errors.Join(errs...)
}

func (c *Config) exporterOrDefault() Exporter {
	if c == nil || c.Exporter == "" {
		return ExporterNone
	}
	return c.Exporter
}

func isValidExporter(exporter Exporter) bool {
	switch exporter {
	case ExporterNone, ExporterStdout, ExporterOTLPGRPC, ExporterOTLPHTTP, ExporterPrometheus:
		return true
	default:
		return false
	}
}

func requiresOTLP(exporter Exporter) bool {
	return exporter == ExporterOTLPGRPC || exporter == ExporterOTLPHTTP
}

func usesPeriodicReader(exporter Exporter) bool {
	return exporter == ExporterStdout || exporter == ExporterOTLPGRPC || exporter == ExporterOTLPHTTP
}

// ServiceConfig 表示服务资源信息。
type ServiceConfig struct {
	// Name 是服务名，会映射为 service.name。
	Name string `json:"name" yaml:"name" mapstructure:"name"`

	// Namespace 是服务命名空间，会映射为 service.namespace。
	Namespace string `json:"namespace" yaml:"namespace" mapstructure:"namespace"`

	// Version 是服务版本，会映射为 service.version。
	Version string `json:"version" yaml:"version" mapstructure:"version"`

	// InstanceID 是服务实例 ID，会映射为 service.instance.id。
	InstanceID string `json:"instance_id" yaml:"instance_id" mapstructure:"instance_id"`

	// Environment 是运行环境，会映射为 deployment.environment.name。
	Environment string `json:"environment" yaml:"environment" mapstructure:"environment"`
}

func (c *ServiceConfig) validate() []error {
	if strings.TrimSpace(c.Name) == "" {
		return []error{errors.New("metrics service.name must not be empty")}
	}
	return nil
}

// ResourceConfig 表示额外 resource attributes。
type ResourceConfig struct {
	Attributes map[string]string `json:"attributes" yaml:"attributes" mapstructure:"attributes"`
}

func (c *ResourceConfig) validate() []error {
	for key := range c.Attributes {
		if strings.TrimSpace(key) == "" {
			return []error{errors.New("metrics resource.attributes key must not be empty")}
		}
	}
	return nil
}

// ReaderConfig 表示周期性导出 reader 配置。
type ReaderConfig struct {
	Interval time.Duration `json:"interval" yaml:"interval" mapstructure:"interval"`
	Timeout  time.Duration `json:"timeout" yaml:"timeout" mapstructure:"timeout"`
}

func (c *ReaderConfig) validate() []error {
	var errs []error
	if c.Interval < 0 {
		errs = append(errs, errors.New("metrics reader.interval must be greater than or equal to 0"))
	}
	if c.Timeout < 0 {
		errs = append(errs, errors.New("metrics reader.timeout must be greater than or equal to 0"))
	}
	return errs
}

func (c *ReaderConfig) intervalOrDefault() time.Duration {
	if c == nil || c.Interval == 0 {
		return defaultReaderInterval
	}
	return c.Interval
}

func (c *ReaderConfig) timeoutOrDefault() time.Duration {
	if c == nil || c.Timeout == 0 {
		return defaultReaderTimeout
	}
	return c.Timeout
}

// OTLPConfig 表示 OTLP exporter 配置。
type OTLPConfig struct {
	Endpoint    string            `json:"endpoint" yaml:"endpoint" mapstructure:"endpoint"`
	Insecure    bool              `json:"insecure" yaml:"insecure" mapstructure:"insecure"`
	Timeout     time.Duration     `json:"timeout" yaml:"timeout" mapstructure:"timeout"`
	Compression Compression       `json:"compression" yaml:"compression" mapstructure:"compression"`
	Headers     map[string]string `json:"headers" yaml:"headers" mapstructure:"headers"`
	URLPath     string            `json:"url_path" yaml:"url_path" mapstructure:"url_path"`
}

func (c *OTLPConfig) validate(exporter Exporter) []error {
	var errs []error
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		errs = append(errs, errors.New("metrics otlp.endpoint must not be empty"))
	}
	if exporter == ExporterOTLPGRPC && (strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://")) {
		errs = append(errs, errors.New("metrics otlp.endpoint for otlp_grpc should be host:port, not http url"))
	}
	if exporter == ExporterOTLPHTTP {
		errs = append(errs, validateHTTPOTLPEndpoint(endpoint)...)
	}
	if c.URLPath != "" {
		urlPath := strings.TrimSpace(c.URLPath)
		switch {
		case urlPath == "":
			errs = append(errs, errors.New("metrics otlp.url_path must not be blank"))
		case exporter != ExporterOTLPHTTP:
			errs = append(errs, errors.New("metrics otlp.url_path requires exporter to be otlp_http"))
		case !strings.HasPrefix(urlPath, "/"):
			errs = append(errs, errors.New("metrics otlp.url_path must start with /"))
		}
	}
	if c.Timeout < 0 {
		errs = append(errs, errors.New("metrics otlp.timeout must be greater than or equal to 0"))
	}
	if c.Compression != "" && !isValidCompression(c.Compression) {
		errs = append(errs, fmt.Errorf("metrics otlp.compression must be one of %q, %q", CompressionNone, CompressionGzip))
	}
	for key := range c.Headers {
		if strings.TrimSpace(key) == "" {
			errs = append(errs, errors.New("metrics otlp.headers key must not be empty"))
			break
		}
	}
	return errs
}

func validateHTTPOTLPEndpoint(endpoint string) []error {
	if endpoint == "" {
		return nil
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		return []error{errors.New("metrics otlp.endpoint for otlp_http must be a valid http or https url")}
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return []error{errors.New("metrics otlp.endpoint for otlp_http must be a valid http or https url")}
	}
	return nil
}

func (c *OTLPConfig) timeoutOrDefault() time.Duration {
	if c == nil || c.Timeout == 0 {
		return defaultOTLPTimeout
	}
	return c.Timeout
}

func (c *OTLPConfig) compressionOrDefault() Compression {
	if c == nil || c.Compression == "" {
		return CompressionGzip
	}
	return c.Compression
}

func isValidCompression(compression Compression) bool {
	switch compression {
	case CompressionNone, CompressionGzip:
		return true
	default:
		return false
	}
}

// PrometheusConfig 表示 Prometheus exporter 配置。
type PrometheusConfig struct {
	Namespace                          string   `json:"namespace" yaml:"namespace" mapstructure:"namespace"`
	WithoutTargetInfo                  bool     `json:"without_target_info" yaml:"without_target_info" mapstructure:"without_target_info"`
	WithoutScopeInfo                   bool     `json:"without_scope_info" yaml:"without_scope_info" mapstructure:"without_scope_info"`
	ResourceAttributesAsConstantLabels []string `json:"resource_attributes_as_constant_labels" yaml:"resource_attributes_as_constant_labels" mapstructure:"resource_attributes_as_constant_labels"`
}

func (c *PrometheusConfig) validate() []error {
	var errs []error
	if c.Namespace != "" && strings.TrimSpace(c.Namespace) == "" {
		errs = append(errs, errors.New("metrics prometheus.namespace must not be blank"))
	}
	for i, key := range c.ResourceAttributesAsConstantLabels {
		if strings.TrimSpace(key) == "" {
			errs = append(errs, fmt.Errorf("metrics prometheus.resource_attributes_as_constant_labels[%d] must not be empty", i))
		}
	}
	return errs
}
