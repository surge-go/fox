package metrics

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	promclient "github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
)

func TestNewNoneExporterSetsGlobalProvider(t *testing.T) {
	provider, err := New(context.Background(), &Config{
		Exporter: ExporterNone,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer shutdownProvider(t, provider)

	if provider == nil || provider.MeterProvider() == nil {
		t.Fatal("New() provider or meter provider is nil")
	}
	if got := otel.GetMeterProvider(); got != provider.MeterProvider() {
		t.Fatalf("otel.GetMeterProvider() = %T, want %T", got, provider.MeterProvider())
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

func TestNewRejectsDuplicateInitialization(t *testing.T) {
	provider1, err := New(context.Background(), &Config{Exporter: ExporterNone})
	if err != nil {
		t.Fatalf("first New() error = %v", err)
	}
	defer shutdownProvider(t, provider1)

	provider2, err := New(context.Background(), &Config{Exporter: ExporterNone})
	if err == nil {
		shutdownProvider(t, provider2)
		t.Fatal("second New() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "already initialized") {
		t.Fatalf("second New() error = %v, want already initialized error", err)
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

	meter := provider.MeterProvider().Meter("test")
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

func TestNewPrometheusExporterUsesInternalRegistry(t *testing.T) {
	provider, err := New(context.Background(), &Config{
		Exporter: ExporterPrometheus,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer shutdownProvider(t, provider)

	if provider.PrometheusGatherer() == nil {
		t.Fatal("PrometheusGatherer() = nil, want internal registry gatherer")
	}

	meter := provider.MeterProvider().Meter("test")
	counter, err := meter.Int64Counter("requests")
	if err != nil {
		t.Fatalf("Int64Counter() error = %v", err)
	}
	counter.Add(context.Background(), 1)

	families, err := provider.PrometheusGatherer().Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if !metricFamilyExists(families, "requests_total") {
		t.Fatalf("Gather() missing requests_total, families = %v", metricFamilyNames(families))
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

func TestShutdownRestoresNoopProvider(t *testing.T) {
	provider, err := New(context.Background(), &Config{Exporter: ExporterNone})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	if got := otel.GetMeterProvider(); reflect.TypeOf(got) != reflect.TypeOf(metricnoop.NewMeterProvider()) {
		t.Fatalf("otel.GetMeterProvider() = %T, want noop provider", got)
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
