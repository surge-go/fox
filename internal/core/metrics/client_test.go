package metrics

import (
	"context"
	"strings"
	"testing"
	"time"

	promclient "github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func TestNewNoneExporter(t *testing.T) {
	provider, err := New(context.Background(), &Config{
		Exporter: ExporterNone,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer shutdownProvider(t, provider)

	if provider == nil {
		t.Fatal("New() provider = nil")
	}
}

func TestNewRejectsNilConfig(t *testing.T) {
	provider, err := New(context.Background(), nil)
	if err == nil {
		shutdownProvider(t, provider)
		t.Fatal("New() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "metrics config is nil") {
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

func TestNewStdoutExporter(t *testing.T) {
	provider, err := New(context.Background(), &Config{
		Exporter: ExporterStdout,
		Reader: &ReaderConfig{
			Interval: time.Hour,
			Timeout:  time.Second,
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer shutdownProvider(t, provider)
}

func TestNewPrometheusExporterWithRegisterer(t *testing.T) {
	registry := promclient.NewRegistry()
	provider, err := NewWithRegisterer(context.Background(), &Config{
		Service: &ServiceConfig{
			Name:        "fox-api",
			Environment: "test",
		},
		Exporter: ExporterPrometheus,
		Prometheus: &PrometheusConfig{
			Namespace: "fox",
			ResourceAttributesAsConstantLabels: []string{
				"service.name",
				"deployment.environment.name",
			},
		},
	}, registry)
	if err != nil {
		t.Fatalf("NewWithRegisterer() error = %v", err)
	}
	defer shutdownProvider(t, provider)

	meter := provider.Meter("test")
	counter, err := meter.Int64Counter("requests", otelmetric.WithDescription("request count"))
	if err != nil {
		t.Fatalf("Int64Counter() error = %v", err)
	}
	counter.Add(context.Background(), 1, otelmetric.WithAttributes(attribute.String("route", "/ping")))

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if !metricFamilyExists(families, "fox_requests_total") {
		t.Fatalf("Gather() missing fox_requests_total, families = %v", metricFamilyNames(families))
	}
	if !metricFamilyExists(families, "target_info") {
		t.Fatalf("Gather() missing target_info, families = %v", metricFamilyNames(families))
	}
}

func TestNewPrometheusExporterWithoutTargetInfo(t *testing.T) {
	registry := promclient.NewRegistry()
	provider, err := NewWithRegisterer(context.Background(), &Config{
		Exporter: ExporterPrometheus,
		Prometheus: &PrometheusConfig{
			WithoutTargetInfo: true,
		},
	}, registry)
	if err != nil {
		t.Fatalf("NewWithRegisterer() error = %v", err)
	}
	defer shutdownProvider(t, provider)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if metricFamilyExists(families, "target_info") {
		t.Fatalf("Gather() has target_info, want disabled")
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

func TestBuildPeriodicReaderOptions(t *testing.T) {
	options := buildPeriodicReaderOptions(&ReaderConfig{
		Interval: time.Second,
		Timeout:  2 * time.Second,
	})
	if got, want := len(options), 2; got != want {
		t.Fatalf("buildPeriodicReaderOptions() len = %d, want %d", got, want)
	}
}

func metricFamilyExists(families []*dto.MetricFamily, name string) bool {
	for _, family := range families {
		if family.GetName() == name {
			return true
		}
	}
	return false
}

func metricFamilyNames(families []*dto.MetricFamily) []string {
	names := make([]string, 0, len(families))
	for _, family := range families {
		names = append(names, family.GetName())
	}
	return names
}

func resourceAttributes(attrs []attribute.KeyValue) map[string]string {
	values := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		values[string(attr.Key)] = attr.Value.AsString()
	}
	return values
}

func shutdownProvider(t *testing.T, provider *sdkmetric.MeterProvider) {
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
