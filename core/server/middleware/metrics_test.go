package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/surge-go/fox/core/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestMetricsRecordsRequestCountAndDuration(t *testing.T) {
	reader, shutdown := setupMetricsTest(t)
	defer shutdown()

	engine := newTestEngine(t)
	engine.Use(Metrics())
	engine.GET("/users/:id", func(c *server.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	requestCount := collectMetric(t, reader, "http.server.request.count")
	sum, ok := requestCount.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("request count data type = %T, want metricdata.Sum[int64]", requestCount.Data)
	}
	if len(sum.DataPoints) != 1 {
		t.Fatalf("request count datapoints len = %d, want 1", len(sum.DataPoints))
	}
	if got, want := sum.DataPoints[0].Value, int64(1); got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	countAttrs := spanAttrs(sum.DataPoints[0].Attributes.ToSlice())
	assertStringAttr(t, countAttrs, "http.request.method", "GET")
	assertIntAttr(t, countAttrs, "http.response.status_code", http.StatusOK)
	assertStringAttr(t, countAttrs, "http.route", "/users/:id")

	requestDuration := collectMetric(t, reader, "http.server.request.duration")
	histogram, ok := requestDuration.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("request duration data type = %T, want metricdata.Histogram[float64]", requestDuration.Data)
	}
	if len(histogram.DataPoints) != 1 {
		t.Fatalf("request duration datapoints len = %d, want 1", len(histogram.DataPoints))
	}
	if got, want := histogram.DataPoints[0].Count, uint64(1); got != want {
		t.Fatalf("request duration count = %d, want %d", got, want)
	}
	durationAttrs := spanAttrs(histogram.DataPoints[0].Attributes.ToSlice())
	assertStringAttr(t, durationAttrs, "http.request.method", "GET")
	assertIntAttr(t, durationAttrs, "http.response.status_code", http.StatusOK)
	assertStringAttr(t, durationAttrs, "http.route", "/users/:id")
}

func TestMetricsSkipFunc(t *testing.T) {
	reader, shutdown := setupMetricsTest(t)
	defer shutdown()

	engine := newTestEngine(t)
	engine.Use(Metrics(MetricsConfig{
		SkipFunc: func(c *server.Context) bool {
			return c.RawRequest().URL.Path == "/health"
		},
	}))
	engine.GET("/health", func(c *server.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if hasMetric(t, reader, "http.server.request.count") {
		t.Fatal("request count metric was recorded for skipped request")
	}
}

func TestMetricsRecordsCustomAttributes(t *testing.T) {
	reader, shutdown := setupMetricsTest(t)
	defer shutdown()

	engine := newTestEngine(t)
	engine.Use(Metrics(
		MetricsConfig{
			AttributesFunc: func(c *server.Context) []attribute.KeyValue {
				return []attribute.KeyValue{attribute.String("app.component", "api")}
			},
		},
		MetricsConfig{
			AttributesFunc: func(c *server.Context) []attribute.KeyValue {
				return []attribute.KeyValue{attribute.String("ignored", "true")}
			},
		},
	))
	engine.GET("/custom", func(c *server.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/custom", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	requestCount := collectMetric(t, reader, "http.server.request.count")
	sum := requestCount.Data.(metricdata.Sum[int64])
	attrs := spanAttrs(sum.DataPoints[0].Attributes.ToSlice())
	assertStringAttr(t, attrs, "app.component", "api")
	if _, ok := attrs["ignored"]; ok {
		t.Fatal("second metrics config was used, want only first config")
	}
}

func TestMetricsCustomAttributesCannotOverrideCoreAttributes(t *testing.T) {
	reader, shutdown := setupMetricsTest(t)
	defer shutdown()

	engine := newTestEngine(t)
	engine.Use(Metrics(MetricsConfig{
		AttributesFunc: func(c *server.Context) []attribute.KeyValue {
			return []attribute.KeyValue{
				attribute.String("http.request.method", "POST"),
				attribute.Int("http.response.status_code", http.StatusTeapot),
				attribute.String("http.route", "/wrong"),
				attribute.String("app.component", "api"),
			}
		},
	}))
	engine.GET("/reserved", func(c *server.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/reserved", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	requestCount := collectMetric(t, reader, "http.server.request.count")
	sum := requestCount.Data.(metricdata.Sum[int64])
	attrs := spanAttrs(sum.DataPoints[0].Attributes.ToSlice())
	assertStringAttr(t, attrs, "http.request.method", "GET")
	assertIntAttr(t, attrs, "http.response.status_code", http.StatusOK)
	assertStringAttr(t, attrs, "http.route", "/reserved")
	assertStringAttr(t, attrs, "app.component", "api")
}

func TestMetricsIgnoresPanickingCustomAttributes(t *testing.T) {
	reader, shutdown := setupMetricsTest(t)
	defer shutdown()

	engine := newTestEngine(t)
	engine.Use(Metrics(MetricsConfig{
		AttributesFunc: func(c *server.Context) []attribute.KeyValue {
			panic("bad metric attribute")
		},
	}))
	engine.GET("/panic-attrs", func(c *server.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/panic-attrs", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	requestCount := collectMetric(t, reader, "http.server.request.count")
	sum := requestCount.Data.(metricdata.Sum[int64])
	attrs := spanAttrs(sum.DataPoints[0].Attributes.ToSlice())
	assertStringAttr(t, attrs, "http.request.method", "GET")
	assertIntAttr(t, attrs, "http.response.status_code", http.StatusOK)
	assertStringAttr(t, attrs, "http.route", "/panic-attrs")
	if _, ok := attrs["app.component"]; ok {
		t.Fatal("custom attribute was recorded after AttributesFunc panic")
	}
}

func TestMetricsDoesNotMaskHandlerPanicWhenCustomAttributesPanic(t *testing.T) {
	reader, shutdown := setupMetricsTest(t)
	defer shutdown()

	var recovered any
	engine := newTestEngine(t)
	engine.Use(func(c *server.Context) {
		defer func() {
			if err := recover(); err != nil {
				recovered = err
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	})
	engine.Use(Metrics(MetricsConfig{
		AttributesFunc: func(c *server.Context) []attribute.KeyValue {
			panic("bad metric attribute")
		},
	}))
	engine.GET("/handler-panic", func(c *server.Context) {
		panic("handler boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/handler-panic", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if recovered != "handler boom" {
		t.Fatalf("recovered panic = %v, want handler boom", recovered)
	}
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	requestCount := collectMetric(t, reader, "http.server.request.count")
	sum := requestCount.Data.(metricdata.Sum[int64])
	attrs := spanAttrs(sum.DataPoints[0].Attributes.ToSlice())
	assertIntAttr(t, attrs, "http.response.status_code", http.StatusInternalServerError)
}

func TestMetricsRecordsServerError(t *testing.T) {
	reader, shutdown := setupMetricsTest(t)
	defer shutdown()

	engine := newTestEngine(t)
	engine.Use(Metrics())
	engine.GET("/fail", func(c *server.Context) {
		c.AbortWithStatus(http.StatusInternalServerError)
	})

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	requestCount := collectMetric(t, reader, "http.server.request.count")
	sum := requestCount.Data.(metricdata.Sum[int64])
	attrs := spanAttrs(sum.DataPoints[0].Attributes.ToSlice())
	assertIntAttr(t, attrs, "http.response.status_code", http.StatusInternalServerError)
}

func TestMetricsRecordsPanicAsServerError(t *testing.T) {
	reader, shutdown := setupMetricsTest(t)
	defer shutdown()

	engine := newTestEngine(t)
	engine.Use(Metrics())
	engine.GET("/panic", func(c *server.Context) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	requestCount := collectMetric(t, reader, "http.server.request.count")
	sum := requestCount.Data.(metricdata.Sum[int64])
	attrs := spanAttrs(sum.DataPoints[0].Attributes.ToSlice())
	assertIntAttr(t, attrs, "http.response.status_code", http.StatusInternalServerError)
}

func setupMetricsTest(t *testing.T) (*sdkmetric.ManualReader, func()) {
	t.Helper()

	oldProvider := otel.GetMeterProvider()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)

	return reader, func() {
		_ = provider.Shutdown(context.Background())
		if oldProvider != nil {
			otel.SetMeterProvider(oldProvider)
		} else {
			otel.SetMeterProvider(metricnoop.NewMeterProvider())
		}
	}
}

func collectMetric(t *testing.T, reader *sdkmetric.ManualReader, name string) metricdata.Metrics {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}

	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				return metric
			}
		}
	}
	t.Fatalf("missing metric %q", name)
	return metricdata.Metrics{}
}

func hasMetric(t *testing.T, reader *sdkmetric.ManualReader, name string) bool {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}

	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				return true
			}
		}
	}
	return false
}
