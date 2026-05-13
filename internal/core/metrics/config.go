package metrics

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Exporter 表示指标数据导出方式。
type Exporter string

const (
	// ExporterNone 表示不导出 metrics 数据，通常用于测试或只创建本地 MeterProvider 的场景。
	ExporterNone Exporter = "none"

	// ExporterPrometheus 表示通过 Prometheus pull 模式暴露指标。
	ExporterPrometheus Exporter = "prometheus"

	// ExporterOTLPGRPC 表示通过 OTLP gRPC 协议导出 metrics 数据。
	ExporterOTLPGRPC Exporter = "otlp_grpc"

	// ExporterOTLPHTTP 表示通过 OTLP HTTP/protobuf 协议导出 metrics 数据。
	ExporterOTLPHTTP Exporter = "otlp_http"

	// ExporterStdout 表示把 metrics 数据输出到 stdout，通常用于本地调试。
	ExporterStdout Exporter = "stdout"
)

// Compression 表示 OTLP exporter 压缩方式。
type Compression string

const (
	// CompressionNone 表示不压缩。
	CompressionNone Compression = "none"

	// CompressionGzip 表示使用 gzip 压缩。
	CompressionGzip Compression = "gzip"
)

// Config 表示 OpenTelemetry metrics 配置。
//
// 该结构体面向“应用配置文件”设计，只保留字符串、数字、布尔值、Duration 和 map
// 等可序列化字段，不包含 metric.Reader、metric.Exporter、prometheus.Registerer、
// resource.Resource 等运行时代码对象。
//
// 常规项目通常只需要配置 Service、Exporter、Prometheus 或 OTLP。Reader、Resource
// 等字段属于按需配置：需要调整周期推送间隔、补充资源标签或细化 Prometheus 命名时再使用。
//
// 配置组字段使用指针是为了区分“整组未配置”和“整组参与覆盖”。例如 Reader 为 nil
// 表示使用 OpenTelemetry 默认周期导出配置；Reader 非 nil 时，组内字段会按 SDK
// 对应字段的语义传递，字段零值通常表示使用实现层默认值。
type Config struct {
	// Service 是当前服务的资源信息。常用配置组。
	// nil 表示使用默认 service.name。
	Service *ServiceConfig `json:"service" yaml:"service" mapstructure:"service"`

	// Exporter 指定 metrics 数据导出方式。常用字段。
	// 为空时建议由 New 使用 ExporterNone，避免本地开发误连 collector 或暴露默认 registry。
	Exporter Exporter `json:"exporter" yaml:"exporter" mapstructure:"exporter"`

	// Prometheus 是 Prometheus exporter 配置。
	// Exporter 为 prometheus 时使用；HTTP 路由注册由应用启动层负责。
	Prometheus *PrometheusConfig `json:"prometheus" yaml:"prometheus" mapstructure:"prometheus"`

	// OTLP 是 OTLP exporter 配置。
	// Exporter 为 otlp_grpc 或 otlp_http 时使用。
	OTLP *OTLPConfig `json:"otlp" yaml:"otlp" mapstructure:"otlp"`

	// Reader 是周期导出 reader 配置。
	// 仅 stdout、otlp_grpc、otlp_http 推送型 exporter 使用；prometheus 使用 pull 模式，不使用该配置。
	Reader *ReaderConfig `json:"reader" yaml:"reader" mapstructure:"reader"`

	// Resource 是额外资源标签配置。按需配置组。
	// nil 表示不追加额外 resource attributes。
	Resource *ResourceConfig `json:"resource" yaml:"resource" mapstructure:"resource"`
}

// Validate 校验 metrics 配置是否满足创建 MeterProvider 的基本要求。
//
// 该方法只做确定性的静态配置校验，不会连接 collector，也不会创建 exporter。
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("metrics config is nil")
	}

	var errs []error
	exporter := c.exporterOrDefault()

	if !isValidExporter(exporter) {
		errs = append(errs, fmt.Errorf("metrics exporter must be one of %q, %q, %q, %q, %q", ExporterNone, ExporterPrometheus, ExporterOTLPGRPC, ExporterOTLPHTTP, ExporterStdout))
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
	if c.Prometheus != nil {
		errs = append(errs, c.Prometheus.validate()...)
	}
	if c.OTLP != nil {
		errs = append(errs, c.OTLP.validate(exporter)...)
	}
	if c.Reader != nil {
		errs = append(errs, c.Reader.validate()...)
	}
	if c.Resource != nil {
		errs = append(errs, c.Resource.validate()...)
	}
	return errors.Join(errs...)
}

func (c *Config) exporterOrDefault() Exporter {
	if c.Exporter == "" {
		return ExporterNone
	}
	return c.Exporter
}

func isValidExporter(exporter Exporter) bool {
	switch exporter {
	case ExporterNone, ExporterPrometheus, ExporterOTLPGRPC, ExporterOTLPHTTP, ExporterStdout:
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
	// 生产环境建议显式配置为稳定服务标识。
	Name string `json:"name" yaml:"name" mapstructure:"name"`

	// Namespace 是服务命名空间，会映射为 service.namespace。
	// 多业务线、多租户或多系统共用 collector 时建议配置。
	Namespace string `json:"namespace" yaml:"namespace" mapstructure:"namespace"`

	// Version 是服务版本，会映射为 service.version。
	// 建议配置为镜像版本、Git tag 或发布版本号。
	Version string `json:"version" yaml:"version" mapstructure:"version"`

	// InstanceID 是服务实例 ID，会映射为 service.instance.id。
	// 通常使用主机名、pod name 或实例唯一编号。
	InstanceID string `json:"instance_id" yaml:"instance_id" mapstructure:"instance_id"`

	// Environment 是运行环境，会映射为 deployment.environment.name。
	// 常见值为 local、dev、test、staging、prod。
	Environment string `json:"environment" yaml:"environment" mapstructure:"environment"`
}

func (c *ServiceConfig) validate() []error {
	var errs []error
	if strings.TrimSpace(c.Name) == "" {
		errs = append(errs, errors.New("metrics service.name must not be empty"))
	}
	return errs
}

// PrometheusConfig 表示 Prometheus exporter 配置。
type PrometheusConfig struct {
	// Namespace 是 Prometheus 指标名前缀。
	// 常用于为同一进程内的业务指标增加统一前缀，例如 fox。
	Namespace string `json:"namespace" yaml:"namespace" mapstructure:"namespace"`

	// WithoutTargetInfo 表示不导出 target_info 指标。
	// target_info 默认携带 resource attributes，便于在 Prometheus 中关联服务元数据。
	WithoutTargetInfo bool `json:"without_target_info" yaml:"without_target_info" mapstructure:"without_target_info"`

	// WithoutScopeInfo 表示不在指标点中附加 instrumentation scope 标签。
	// 指标标签基数敏感时可以开启。
	WithoutScopeInfo bool `json:"without_scope_info" yaml:"without_scope_info" mapstructure:"without_scope_info"`

	// ResourceAttributesAsConstantLabels 指定哪些 resource attributes 作为常量标签附加到所有指标。
	// 只建议放 env、region、service.namespace 等低基数稳定标签，避免 pod、instance 等高基数字段。
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

// OTLPConfig 表示 OTLP exporter 配置。
type OTLPConfig struct {
	// Endpoint 是 OpenTelemetry Collector 或后端的地址。
	// gRPC 常见格式为 host:4317；HTTP 常见格式为 http://host:4318。
	Endpoint string `json:"endpoint" yaml:"endpoint" mapstructure:"endpoint"`

	// URLPath 是 OTLP HTTP 请求路径，仅 otlp_http 使用。
	// 为空时使用 exporter 默认路径。
	URLPath string `json:"url_path" yaml:"url_path" mapstructure:"url_path"`

	// Insecure 表示是否使用明文连接。
	// 内网 collector 常用 true；公网或跨网络场景建议使用 TLS。
	Insecure bool `json:"insecure" yaml:"insecure" mapstructure:"insecure"`

	// Headers 是发送给 collector 的固定请求头。
	// 常用于 token、租户 ID 或后端鉴权信息。
	Headers map[string]string `json:"headers" yaml:"headers" mapstructure:"headers"`

	// Timeout 是单次导出超时时间。
	// 0 表示使用 exporter 默认值。
	Timeout time.Duration `json:"timeout" yaml:"timeout" mapstructure:"timeout"`

	// Compression 指定压缩方式，支持 none、gzip。
	// 空值表示使用 exporter 默认值。
	Compression Compression `json:"compression" yaml:"compression" mapstructure:"compression"`
}

func (c *OTLPConfig) validate(exporter Exporter) []error {
	var errs []error
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		errs = append(errs, errors.New("metrics otlp.endpoint must not be empty"))
	}
	if exporter == ExporterOTLPHTTP {
		errs = append(errs, validateOTLPHTTPEndpoint(endpoint)...)
	}
	if strings.TrimSpace(c.URLPath) != "" {
		if exporter != ExporterOTLPHTTP {
			errs = append(errs, errors.New("metrics otlp.url_path requires exporter to be otlp_http"))
		}
		if !strings.HasPrefix(strings.TrimSpace(c.URLPath), "/") {
			errs = append(errs, errors.New("metrics otlp.url_path must start with /"))
		}
	} else if c.URLPath != "" {
		errs = append(errs, errors.New("metrics otlp.url_path must not be blank"))
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
	if exporter == ExporterOTLPGRPC && strings.HasPrefix(endpoint, "http") {
		errs = append(errs, errors.New("metrics otlp.endpoint for otlp_grpc should be host:port, not http url"))
	}
	return errs
}

func validateOTLPHTTPEndpoint(endpoint string) []error {
	if endpoint == "" || !(strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://")) {
		return nil
	}

	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return []error{errors.New("metrics otlp.endpoint for otlp_http must be a valid http or https url")}
	}
	return nil
}

func isValidCompression(compression Compression) bool {
	switch compression {
	case CompressionNone, CompressionGzip:
		return true
	default:
		return false
	}
}

// ReaderConfig 表示周期导出 Reader 配置。
type ReaderConfig struct {
	// Interval 是两次 metrics 采集并导出的间隔。
	// 0 表示使用 OpenTelemetry 默认值。
	Interval time.Duration `json:"interval" yaml:"interval" mapstructure:"interval"`

	// Timeout 是单次采集和导出的超时时间。
	// 0 表示使用 OpenTelemetry 默认值。
	Timeout time.Duration `json:"timeout" yaml:"timeout" mapstructure:"timeout"`
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

// ResourceConfig 表示额外资源标签配置。
type ResourceConfig struct {
	// Attributes 是附加到 MeterProvider resource 上的静态标签。
	// 常用于 region、zone、cluster、team 等稳定维度。
	Attributes map[string]string `json:"attributes" yaml:"attributes" mapstructure:"attributes"`
}

func (c *ResourceConfig) validate() []error {
	var errs []error
	for key := range c.Attributes {
		if strings.TrimSpace(key) == "" {
			errs = append(errs, errors.New("metrics resource.attributes key must not be empty"))
			break
		}
	}
	return errs
}
