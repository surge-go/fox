package metrics

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

func TestConfigValidatePrometheus(t *testing.T) {
	cfg := &Config{
		Service: &ServiceConfig{
			Name:        "fox-api",
			Namespace:   "surge",
			Version:     "v1.0.0",
			InstanceID:  "pod-1",
			Environment: "prod",
		},
		Exporter: ExporterPrometheus,
		Prometheus: &PrometheusConfig{
			Namespace:         "fox",
			WithoutTargetInfo: true,
			WithoutScopeInfo:  true,
			ResourceAttributesAsConstantLabels: []string{
				"deployment.environment.name",
				"region",
			},
		},
		Resource: &ResourceConfig{
			Attributes: map[string]string{
				"region": "cn-shenzhen",
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateOTLP(t *testing.T) {
	cfg := &Config{
		Exporter: ExporterOTLPHTTP,
		OTLP: &OTLPConfig{
			Endpoint:    "http://otel-collector:4318",
			URLPath:     "/v1/metrics",
			Insecure:    true,
			Timeout:     5 * time.Second,
			Compression: CompressionGzip,
			Headers: map[string]string{
				"x-tenant-id": "fox",
			},
		},
		Reader: &ReaderConfig{
			Interval: 10 * time.Second,
			Timeout:  5 * time.Second,
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
		Prometheus: &PrometheusConfig{
			Namespace: " ",
			ResourceAttributesAsConstantLabels: []string{
				"",
			},
		},
		OTLP: &OTLPConfig{
			Endpoint:    " ",
			Timeout:     -time.Second,
			Compression: Compression("snappy"),
			Headers: map[string]string{
				"": "empty",
			},
		},
		Reader: &ReaderConfig{
			Interval: -time.Second,
			Timeout:  -time.Second,
		},
		Resource: &ResourceConfig{
			Attributes: map[string]string{
				"": "empty",
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}

	wantContains := []string{
		"metrics exporter must be one of",
		"metrics otlp config requires exporter to be otlp_grpc or otlp_http",
		"metrics prometheus config requires exporter to be prometheus",
		"metrics reader config requires exporter to be stdout, otlp_grpc, or otlp_http",
		"metrics service.name must not be empty",
		"metrics prometheus.namespace must not be blank",
		"metrics prometheus.resource_attributes_as_constant_labels[0] must not be empty",
		"metrics otlp.endpoint must not be empty",
		"metrics otlp.timeout must be greater than or equal to 0",
		"metrics otlp.compression must be one of",
		"metrics otlp.headers key must not be empty",
		"metrics reader.interval must be greater than or equal to 0",
		"metrics reader.timeout must be greater than or equal to 0",
		"metrics resource.attributes key must not be empty",
	}
	for _, want := range wantContains {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Validate() error = %v, want to contain %q", err, want)
		}
	}
}

func TestConfigValidateRequiresOTLPConfig(t *testing.T) {
	cfg := &Config{
		Exporter: ExporterOTLPGRPC,
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "metrics otlp config is required") {
		t.Fatalf("Validate() error = %v, want otlp required error", err)
	}
}

func TestConfigValidateGRPCEndpointShouldNotBeHTTPURL(t *testing.T) {
	cfg := &Config{
		Exporter: ExporterOTLPGRPC,
		OTLP: &OTLPConfig{
			Endpoint: "http://otel-collector:4317",
		},
		Reader: &ReaderConfig{},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "metrics otlp.endpoint for otlp_grpc should be host:port") {
		t.Fatalf("Validate() error = %v, want grpc endpoint error", err)
	}
}

func TestConfigValidateHTTPOnlyURLPath(t *testing.T) {
	cfg := &Config{
		Exporter: ExporterOTLPGRPC,
		OTLP: &OTLPConfig{
			Endpoint: "otel-collector:4317",
			URLPath:  "/v1/metrics",
		},
		Reader: &ReaderConfig{},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "metrics otlp.url_path requires exporter to be otlp_http") {
		t.Fatalf("Validate() error = %v, want url path error", err)
	}
}

func TestConfigValidateHTTPURLPathMustNotBeBlank(t *testing.T) {
	cfg := &Config{
		Exporter: ExporterOTLPHTTP,
		OTLP: &OTLPConfig{
			Endpoint: "http://otel-collector:4318",
			URLPath:  " ",
		},
		Reader: &ReaderConfig{},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "metrics otlp.url_path must not be blank") {
		t.Fatalf("Validate() error = %v, want blank url path error", err)
	}
}

func TestConfigValidateHTTPURLPathMustStartWithSlash(t *testing.T) {
	cfg := &Config{
		Exporter: ExporterOTLPHTTP,
		OTLP: &OTLPConfig{
			Endpoint: "http://otel-collector:4318",
			URLPath:  "v1/metrics",
		},
		Reader: &ReaderConfig{},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "metrics otlp.url_path must start with /") {
		t.Fatalf("Validate() error = %v, want url path slash error", err)
	}
}

func TestConfigValidateOTLPHTTPURLMustBeValid(t *testing.T) {
	cfg := &Config{
		Exporter: ExporterOTLPHTTP,
		OTLP: &OTLPConfig{
			Endpoint: "http://",
		},
		Reader: &ReaderConfig{},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "metrics otlp.endpoint for otlp_http must be a valid http or https url") {
		t.Fatalf("Validate() error = %v, want invalid endpoint url error", err)
	}
}
