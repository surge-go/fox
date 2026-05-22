package tracing

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

func TestNewNoneExporter(t *testing.T) {
	provider, err := New(context.Background(), &Config{
		Exporter: ExporterNone,
		Sampler: &SamplerConfig{
			Type: SamplerAlwaysOn,
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer shutdownProvider(t, provider)

	_, span := provider.Tracer("test").Start(context.Background(), "operation")
	defer span.End()

	if !span.SpanContext().IsValid() {
		t.Fatal("span context is invalid")
	}
}

func TestNewStdoutExporter(t *testing.T) {
	provider, err := New(context.Background(), &Config{
		Exporter: ExporterStdout,
		Sampler: &SamplerConfig{
			Type: SamplerAlwaysOn,
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer shutdownProvider(t, provider)
}

func TestNewRejectsNilConfig(t *testing.T) {
	provider, err := New(context.Background(), nil)
	if err == nil {
		shutdownProvider(t, provider)
		t.Fatal("New() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "tracing config is nil") {
		t.Fatalf("New() error = %v, want nil config error", err)
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	provider, err := New(context.Background(), &Config{
		Exporter: ExporterOTLPGRPC,
	})
	if err == nil {
		shutdownProvider(t, provider)
		t.Fatal("New() error = nil, want error")
	}
}

func TestNewAllowsNilContext(t *testing.T) {
	provider, err := New(nil, &Config{
		Exporter: ExporterNone,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer shutdownProvider(t, provider)
}

func TestNewRegistersGlobalProviderAndShutdownClearsIt(t *testing.T) {
	provider, err := New(context.Background(), &Config{
		Exporter: ExporterNone,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	global, ok := GlobalTracerProvider()
	if !ok {
		t.Fatal("GlobalTracerProvider() ok = false, want true")
	}
	if global != provider.TracerProvider {
		t.Fatal("GlobalTracerProvider() did not return provider created by New")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := provider.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	if _, ok := GlobalTracerProvider(); ok {
		t.Fatal("GlobalTracerProvider() ok = true after Shutdown, want false")
	}
}

func TestNewRejectsDuplicateInitialization(t *testing.T) {
	first, err := New(context.Background(), &Config{
		Exporter: ExporterNone,
	})
	if err != nil {
		t.Fatalf("first New() error = %v", err)
	}
	defer shutdownProvider(t, first)

	second, err := New(context.Background(), &Config{
		Exporter: ExporterNone,
	})
	if err == nil {
		shutdownProvider(t, second)
		t.Fatal("second New() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "already initialized") {
		t.Fatalf("second New() error = %v, want already initialized error", err)
	}
}

func TestProviderShutdownIsIdempotent(t *testing.T) {
	provider, err := New(context.Background(), &Config{
		Exporter: ExporterNone,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := provider.Shutdown(ctx); err != nil {
		t.Fatalf("first Shutdown() error = %v", err)
	}
	if err := provider.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
}

func TestBuildSampler(t *testing.T) {
	tests := []struct {
		name string
		cfg  *SamplerConfig
	}{
		{name: "always_on", cfg: &SamplerConfig{Type: SamplerAlwaysOn}},
		{name: "always_off", cfg: &SamplerConfig{Type: SamplerAlwaysOff}},
		{name: "ratio", cfg: &SamplerConfig{Type: SamplerTraceIDRatio, Ratio: 0.5}},
		{name: "parent_on", cfg: &SamplerConfig{Type: SamplerParentBasedAlwaysOn}},
		{name: "parent_ratio", cfg: &SamplerConfig{Type: SamplerParentBasedTraceIDRatio, Ratio: 0.5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sampler := buildSampler(tt.cfg)
			if sampler == nil {
				t.Fatal("buildSampler() = nil")
			}
		})
	}
}

func TestBuildResource(t *testing.T) {
	res, err := buildResource(&Config{
		Service: &ServiceConfig{
			Name:        "fox-api",
			Namespace:   "surge",
			Version:     "v1.0.0",
			InstanceID:  "pod-1",
			Environment: "prod",
		},
		Resource: &ResourceConfig{
			Attributes: map[string]string{
				"region": "cn-shenzhen",
			},
		},
	})
	if err != nil {
		t.Fatalf("buildResource() error = %v", err)
	}

	attrs := resourceAttributes(res.Attributes())
	want := map[string]string{
		"service.name":                "fox-api",
		"service.namespace":           "surge",
		"service.version":             "v1.0.0",
		"service.instance.id":         "pod-1",
		"deployment.environment.name": "prod",
		"region":                      "cn-shenzhen",
	}
	for key, value := range want {
		if attrs[key] != value {
			t.Fatalf("resource attribute %s = %q, want %q", key, attrs[key], value)
		}
	}
}

func TestBuildBatchOptions(t *testing.T) {
	options := buildBatchOptions(&BatchConfig{
		MaxQueueSize:       100,
		BatchTimeout:       time.Second,
		ExportTimeout:      2 * time.Second,
		MaxExportBatchSize: 50,
	})
	if got, want := len(options), 4; got != want {
		t.Fatalf("buildBatchOptions() len = %d, want %d", got, want)
	}
}

func resourceAttributes(attrs []attribute.KeyValue) map[string]string {
	values := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		values[string(attr.Key)] = attr.Value.AsString()
	}
	return values
}

func shutdownProvider(t *testing.T, provider *Provider) {
	t.Helper()
	if provider == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := provider.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}
