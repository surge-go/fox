package tracing

import (
	"strings"
	"testing"
	"time"
)

func TestConfigValidateDefaults(t *testing.T) {
	cfg := &Config{}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateOTLP(t *testing.T) {
	cfg := &Config{
		Service: &ServiceConfig{
			Name:        "fox-api",
			Namespace:   "surge",
			Version:     "v1.0.0",
			InstanceID:  "pod-1",
			Environment: "prod",
		},
		Exporter: ExporterOTLPGRPC,
		OTLP: &OTLPConfig{
			Endpoint:    "otel-collector:4317",
			Insecure:    true,
			Timeout:     5 * time.Second,
			Compression: CompressionGzip,
			Headers: map[string]string{
				"x-tenant-id": "fox",
			},
		},
		Sampler: &SamplerConfig{
			Type:  SamplerParentBasedTraceIDRatio,
			Ratio: 0.1,
		},
		Resource: &ResourceConfig{
			Attributes: map[string]string{
				"region": "cn-shenzhen",
			},
		},
		Batch: &BatchConfig{
			MaxQueueSize:       2048,
			BatchTimeout:       5 * time.Second,
			ExportTimeout:      30 * time.Second,
			MaxExportBatchSize: 512,
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateInvalidConfig(t *testing.T) {
	cfg := &Config{
		Service: &ServiceConfig{
			Name: " ",
		},
		Exporter: Exporter("zipkin"),
		OTLP: &OTLPConfig{
			Endpoint:    " ",
			Timeout:     -time.Second,
			Compression: Compression("snappy"),
			Headers: map[string]string{
				"": "empty",
			},
		},
		Sampler: &SamplerConfig{
			Type:  Sampler("custom"),
			Ratio: 2,
		},
		Resource: &ResourceConfig{
			Attributes: map[string]string{
				"": "empty",
			},
		},
		Batch: &BatchConfig{
			MaxQueueSize:       1,
			BatchTimeout:       -time.Second,
			ExportTimeout:      -time.Second,
			MaxExportBatchSize: 2,
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}

	wantContains := []string{
		"tracing exporter must be one of",
		"tracing otlp config requires exporter to be otlp_grpc or otlp_http",
		"tracing service.name must not be empty",
		"tracing otlp.endpoint must not be empty",
		"tracing otlp.timeout must be greater than or equal to 0",
		"tracing otlp.compression must be one of",
		"tracing otlp.headers key must not be empty",
		"tracing sampler.type must be one of",
		"tracing sampler.ratio must be between 0 and 1",
		"tracing resource.attributes key must not be empty",
		"tracing batch.batch_timeout must be greater than or equal to 0",
		"tracing batch.export_timeout must be greater than or equal to 0",
		"tracing batch.max_export_batch_size must be less than or equal to max_queue_size",
	}
	for _, want := range wantContains {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Validate() error = %v, want to contain %q", err, want)
		}
	}
}

func TestConfigValidateRequiresOTLPConfig(t *testing.T) {
	cfg := &Config{
		Exporter: ExporterOTLPHTTP,
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "tracing otlp config is required") {
		t.Fatalf("Validate() error = %v, want otlp required error", err)
	}
}

func TestConfigValidateGRPCEndpointShouldNotBeHTTPURL(t *testing.T) {
	cfg := &Config{
		Exporter: ExporterOTLPGRPC,
		OTLP: &OTLPConfig{
			Endpoint: "http://otel-collector:4317",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "tracing otlp.endpoint for otlp_grpc should be host:port") {
		t.Fatalf("Validate() error = %v, want grpc endpoint error", err)
	}
}

func TestConfigValidateGRPCEndpointMayStartWithHTTPText(t *testing.T) {
	cfg := &Config{
		Exporter: ExporterOTLPGRPC,
		OTLP: &OTLPConfig{
			Endpoint: "http-collector:4317",
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestConfigValidateAllowsZeroRatioSampler(t *testing.T) {
	cfg := &Config{
		Sampler: &SamplerConfig{
			Type: SamplerTraceIDRatio,
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
