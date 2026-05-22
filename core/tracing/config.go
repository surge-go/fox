package tracing

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Exporter 表示链路追踪数据导出方式。
type Exporter string

const (
	// ExporterNone 表示不导出 trace 数据，通常用于测试或只本地创建 span 的场景。
	ExporterNone Exporter = "none"

	// ExporterOTLPGRPC 表示通过 OTLP gRPC 协议导出 trace 数据。
	ExporterOTLPGRPC Exporter = "otlp_grpc"

	// ExporterOTLPHTTP 表示通过 OTLP HTTP/protobuf 协议导出 trace 数据。
	ExporterOTLPHTTP Exporter = "otlp_http"

	// ExporterStdout 表示把 trace 数据输出到 stdout，通常用于本地调试。
	ExporterStdout Exporter = "stdout"
)

// Sampler 表示 trace 采样策略。
type Sampler string

const (
	// SamplerAlwaysOn 表示所有 trace 都采样。
	SamplerAlwaysOn Sampler = "always_on"

	// SamplerAlwaysOff 表示所有 trace 都不采样。
	SamplerAlwaysOff Sampler = "always_off"

	// SamplerTraceIDRatio 表示按 trace id 比例采样。
	SamplerTraceIDRatio Sampler = "trace_id_ratio"

	// SamplerParentBasedAlwaysOn 表示尊重父 span 采样决策，没有父 span 时默认采样。
	SamplerParentBasedAlwaysOn Sampler = "parent_based_always_on"

	// SamplerParentBasedTraceIDRatio 表示尊重父 span 采样决策，没有父 span 时按比例采样。
	SamplerParentBasedTraceIDRatio Sampler = "parent_based_trace_id_ratio"
)

// Compression 表示 OTLP exporter 压缩方式。
type Compression string

const (
	// CompressionNone 表示不压缩。
	CompressionNone Compression = "none"

	// CompressionGzip 表示使用 gzip 压缩。
	CompressionGzip Compression = "gzip"
)

// Config 表示 OpenTelemetry tracing 配置。
//
// 该结构体面向“应用配置文件”设计，只保留字符串、数字、布尔值、Duration 和 map
// 等可序列化字段，不包含 trace.TracerProvider、sdktrace.SpanExporter、resource.Resource
// 等运行时代码对象。
//
// 常规项目通常只需要配置 Service、Exporter、OTLP 和 Sampler。Resource、Batch 等字段
// 属于按需配置：需要补充资源标签、调整导出队列或优化批量上报行为时再使用。
//
// 配置组字段使用指针是为了区分“整组未配置”和“整组参与覆盖”。例如 OTLP 为 nil
// 表示不配置 OTLP exporter 参数；OTLP 非 nil 时，组内字段会按 OpenTelemetry exporter
// 语义传递，字段零值通常表示使用实现层默认值。
type Config struct {
	// Service 是当前服务的资源信息。常用配置组。
	// nil 表示使用默认 service.name。
	Service *ServiceConfig `json:"service" yaml:"service" mapstructure:"service"`

	// Exporter 指定 trace 数据导出方式。常用字段。
	// 为空时建议由 New 使用 ExporterNone，避免本地开发误连 collector。
	Exporter Exporter `json:"exporter" yaml:"exporter" mapstructure:"exporter"`

	// OTLP 是 OTLP exporter 配置。常用配置组。
	// Exporter 为 otlp_grpc 或 otlp_http 时使用。
	OTLP *OTLPConfig `json:"otlp" yaml:"otlp" mapstructure:"otlp"`

	// Sampler 是 trace 采样配置。常用配置组。
	// nil 表示使用 OpenTelemetry 默认采样策略。
	Sampler *SamplerConfig `json:"sampler" yaml:"sampler" mapstructure:"sampler"`

	// Resource 是额外资源标签配置。按需配置组。
	// nil 表示不追加额外 resource attributes。
	Resource *ResourceConfig `json:"resource" yaml:"resource" mapstructure:"resource"`

	// Batch 是 BatchSpanProcessor 配置。按需配置组。
	// nil 表示使用 OpenTelemetry 默认批量导出配置。
	Batch *BatchConfig `json:"batch" yaml:"batch" mapstructure:"batch"`
}

// Validate 校验 tracing 配置是否满足创建 TracerProvider 的基本要求。
//
// 该方法只做确定性的静态配置校验，不会连接 collector，也不会创建 exporter。
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("tracing config is nil")
	}

	var errs []error
	exporter := c.exporterOrDefault()

	if !isValidExporter(exporter) {
		errs = append(errs, fmt.Errorf("tracing exporter must be one of %q, %q, %q, %q", ExporterNone, ExporterOTLPGRPC, ExporterOTLPHTTP, ExporterStdout))
	}
	if requiresOTLP(exporter) && c.OTLP == nil {
		errs = append(errs, errors.New("tracing otlp config is required when exporter is otlp_grpc or otlp_http"))
	}
	if !requiresOTLP(exporter) && c.OTLP != nil {
		errs = append(errs, errors.New("tracing otlp config requires exporter to be otlp_grpc or otlp_http"))
	}
	if c.Service != nil {
		errs = append(errs, c.Service.validate()...)
	}
	if c.OTLP != nil {
		errs = append(errs, c.OTLP.validate(exporter)...)
	}
	if c.Sampler != nil {
		errs = append(errs, c.Sampler.validate()...)
	}
	if c.Resource != nil {
		errs = append(errs, c.Resource.validate()...)
	}
	if c.Batch != nil {
		errs = append(errs, c.Batch.validate()...)
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
	case ExporterNone, ExporterOTLPGRPC, ExporterOTLPHTTP, ExporterStdout:
		return true
	default:
		return false
	}
}

func requiresOTLP(exporter Exporter) bool {
	return exporter == ExporterOTLPGRPC || exporter == ExporterOTLPHTTP
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
		errs = append(errs, errors.New("tracing service.name must not be empty"))
	}
	return errs
}

// OTLPConfig 表示 OTLP exporter 配置。
type OTLPConfig struct {
	// Endpoint 是 OpenTelemetry Collector 或后端的地址。
	// gRPC 常见格式为 host:4317；HTTP 常见格式为 http://host:4318。
	Endpoint string `json:"endpoint" yaml:"endpoint" mapstructure:"endpoint"`

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
	if strings.TrimSpace(c.Endpoint) == "" {
		errs = append(errs, errors.New("tracing otlp.endpoint must not be empty"))
	}
	if c.Timeout < 0 {
		errs = append(errs, errors.New("tracing otlp.timeout must be greater than or equal to 0"))
	}
	if c.Compression != "" && !isValidCompression(c.Compression) {
		errs = append(errs, fmt.Errorf("tracing otlp.compression must be one of %q, %q", CompressionNone, CompressionGzip))
	}
	for key := range c.Headers {
		if strings.TrimSpace(key) == "" {
			errs = append(errs, errors.New("tracing otlp.headers key must not be empty"))
			break
		}
	}
	if exporter == ExporterOTLPGRPC && (strings.HasPrefix(strings.TrimSpace(c.Endpoint), "http://") || strings.HasPrefix(strings.TrimSpace(c.Endpoint), "https://")) {
		errs = append(errs, errors.New("tracing otlp.endpoint for otlp_grpc should be host:port, not http url"))
	}
	return errs
}

func isValidCompression(compression Compression) bool {
	switch compression {
	case CompressionNone, CompressionGzip:
		return true
	default:
		return false
	}
}

// SamplerConfig 表示 trace 采样配置。
type SamplerConfig struct {
	// Type 是采样策略。
	// 生产环境通常使用 parent_based_trace_id_ratio。
	Type Sampler `json:"type" yaml:"type" mapstructure:"type"`

	// Ratio 是 trace_id_ratio 和 parent_based_trace_id_ratio 的采样比例。
	// 合法范围为 0 到 1；例如 0.1 表示采样 10%，0 表示不采样新的根 trace。
	Ratio float64 `json:"ratio" yaml:"ratio" mapstructure:"ratio"`
}

func (c *SamplerConfig) validate() []error {
	var errs []error
	sampler := c.typeOrDefault()
	if !isValidSampler(sampler) {
		errs = append(errs, fmt.Errorf("tracing sampler.type must be one of %q, %q, %q, %q, %q", SamplerAlwaysOn, SamplerAlwaysOff, SamplerTraceIDRatio, SamplerParentBasedAlwaysOn, SamplerParentBasedTraceIDRatio))
	}
	if c.Ratio < 0 || c.Ratio > 1 {
		errs = append(errs, errors.New("tracing sampler.ratio must be between 0 and 1"))
	}
	return errs
}

func (c *SamplerConfig) typeOrDefault() Sampler {
	if c.Type == "" {
		return SamplerParentBasedAlwaysOn
	}
	return c.Type
}

func isValidSampler(sampler Sampler) bool {
	switch sampler {
	case SamplerAlwaysOn, SamplerAlwaysOff, SamplerTraceIDRatio, SamplerParentBasedAlwaysOn, SamplerParentBasedTraceIDRatio:
		return true
	default:
		return false
	}
}

func requiresRatio(sampler Sampler) bool {
	return sampler == SamplerTraceIDRatio || sampler == SamplerParentBasedTraceIDRatio
}

// ResourceConfig 表示额外资源标签配置。
type ResourceConfig struct {
	// Attributes 是附加到 TracerProvider resource 上的静态标签。
	// 常用于 region、zone、cluster、team 等稳定维度。
	Attributes map[string]string `json:"attributes" yaml:"attributes" mapstructure:"attributes"`
}

func (c *ResourceConfig) validate() []error {
	var errs []error
	for key := range c.Attributes {
		if strings.TrimSpace(key) == "" {
			errs = append(errs, errors.New("tracing resource.attributes key must not be empty"))
			break
		}
	}
	return errs
}

// BatchConfig 表示 BatchSpanProcessor 配置。
type BatchConfig struct {
	// MaxQueueSize 是待导出 span 队列最大长度。
	// 0 表示使用 OpenTelemetry 默认值。
	MaxQueueSize int `json:"max_queue_size" yaml:"max_queue_size" mapstructure:"max_queue_size"`

	// BatchTimeout 是批量导出等待时间。
	// 0 表示使用 OpenTelemetry 默认值。
	BatchTimeout time.Duration `json:"batch_timeout" yaml:"batch_timeout" mapstructure:"batch_timeout"`

	// ExportTimeout 是单次批量导出超时时间。
	// 0 表示使用 OpenTelemetry 默认值。
	ExportTimeout time.Duration `json:"export_timeout" yaml:"export_timeout" mapstructure:"export_timeout"`

	// MaxExportBatchSize 是单次最多导出的 span 数量。
	// 0 表示使用 OpenTelemetry 默认值。
	MaxExportBatchSize int `json:"max_export_batch_size" yaml:"max_export_batch_size" mapstructure:"max_export_batch_size"`
}

func (c *BatchConfig) validate() []error {
	var errs []error
	if c.MaxQueueSize < 0 {
		errs = append(errs, errors.New("tracing batch.max_queue_size must be greater than or equal to 0"))
	}
	if c.BatchTimeout < 0 {
		errs = append(errs, errors.New("tracing batch.batch_timeout must be greater than or equal to 0"))
	}
	if c.ExportTimeout < 0 {
		errs = append(errs, errors.New("tracing batch.export_timeout must be greater than or equal to 0"))
	}
	if c.MaxExportBatchSize < 0 {
		errs = append(errs, errors.New("tracing batch.max_export_batch_size must be greater than or equal to 0"))
	}
	if c.MaxQueueSize > 0 && c.MaxExportBatchSize > c.MaxQueueSize {
		errs = append(errs, errors.New("tracing batch.max_export_batch_size must be less than or equal to max_queue_size"))
	}
	return errs
}
