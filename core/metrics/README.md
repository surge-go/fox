# Fox Metrics

`core/metrics` 负责初始化应用级 OpenTelemetry Metrics 管线，为数据库、Redis 和业务代码中的指标采集点提供统一出口。

该包只负责：

- 创建并设置全局 OpenTelemetry `MeterProvider`。
- 根据配置创建 metrics exporter / reader。
- 托管 `MeterProvider` 生命周期，提供 `Shutdown`。
- 在 Prometheus 模式下提供可被应用层暴露的 `Gatherer`。

该包不负责定义业务指标，不直接采集数据库或 Redis 指标，也不启动 HTTP 服务。

## 快速开始

```go
package main

import (
	"context"
	"log"

	"github.com/surge-go/fox/core/metrics"
)

func main() {
	ctx := context.Background()

	provider, err := metrics.New(ctx, &metrics.Config{
		Service: &metrics.ServiceConfig{
			Name:        "fox-api",
			Namespace:   "surge",
			Version:     "v1.0.0",
			Environment: "prod",
		},
		Exporter: metrics.ExporterOTLPGRPC,
		OTLP: &metrics.OTLPConfig{
			Endpoint: "otel-collector:4317",
			Insecure: true,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer provider.Shutdown(ctx)

	// 在这里初始化 database / redis / server / 业务模块。
}
```

`metrics.New` 会把创建出的 `MeterProvider` 设置为 OpenTelemetry 全局 provider。依赖全局 provider 的 instrumentation 应在它之后初始化。

## 初始化顺序

`core/database` 和 `core/redis` 的 `MetricsEnabled` 只表示注册采集点，不表示指标已经能被导出。

推荐顺序：

```go
metricsProvider, err := metrics.New(ctx, metricsCfg)
if err != nil {
	return err
}
defer metricsProvider.Shutdown(ctx)

db, err := database.NewClient(databaseCfgWithMetricsEnabled)
if err != nil {
	return err
}

redisClient, err := redis.NewClient(redisCfgWithMetricsEnabled)
if err != nil {
	return err
}
```

如果没有先初始化 `core/metrics`，即使 database / redis 的 `MetricsEnabled` 设置为 `true`，指标也可能只注册到默认 noop provider 上，无法导出。

## Exporter

支持的导出方式：

| Exporter | 用途 |
| --- | --- |
| `none` | 创建可用的本地 `MeterProvider`，但不导出指标 |
| `stdout` | 输出到 stdout，适合本地调试 |
| `otlp_grpc` | 通过 OTLP gRPC 上报到 OpenTelemetry Collector |
| `otlp_http` | 通过 OTLP HTTP/protobuf 上报到 Collector 或后端 |
| `prometheus` | 通过 Prometheus pull 模式暴露指标 |

默认 `Exporter` 为空时等同于 `none`。

## OTLP gRPC

```go
provider, err := metrics.New(ctx, &metrics.Config{
	Service: &metrics.ServiceConfig{
		Name: "fox-api",
	},
	Exporter: metrics.ExporterOTLPGRPC,
	OTLP: &metrics.OTLPConfig{
		Endpoint:    "otel-collector:4317",
		Insecure:    true,
		Timeout:     5 * time.Second,
		Compression: metrics.CompressionGzip,
	},
	Reader: &metrics.ReaderConfig{
		Interval: 15 * time.Second,
		Timeout:  5 * time.Second,
	},
})
```

`otlp_grpc` 的 `Endpoint` 应使用 `host:port`，不要包含 `http://` 或 `https://`。

## OTLP HTTP

```go
provider, err := metrics.New(ctx, &metrics.Config{
	Service: &metrics.ServiceConfig{
		Name: "fox-api",
	},
	Exporter: metrics.ExporterOTLPHTTP,
	OTLP: &metrics.OTLPConfig{
		Endpoint: "http://otel-collector:4318",
		URLPath:  "/v1/metrics",
		Insecure: true,
		Headers: map[string]string{
			"x-tenant-id": "fox",
		},
	},
})
```

`otlp_http` 的 `Endpoint` 必须是合法的 `http://` 或 `https://` URL。

## Prometheus

`ExporterPrometheus` 会创建 OpenTelemetry Prometheus exporter。应用层负责把 `PrometheusGatherer()` 暴露为 HTTP `/metrics`。

```go
provider, err := metrics.New(ctx, &metrics.Config{
	Service: &metrics.ServiceConfig{
		Name:        "fox-api",
		Environment: "prod",
	},
	Exporter: metrics.ExporterPrometheus,
	Prometheus: &metrics.PrometheusConfig{
		Namespace: "fox",
		ResourceAttributesAsConstantLabels: []string{
			"service.name",
			"deployment.environment.name",
		},
	},
})
if err != nil {
	return err
}

if gatherer := provider.PrometheusGatherer(); gatherer != nil {
	http.Handle("/metrics", promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}))
}
```

默认不传自定义 registerer 时，`core/metrics` 会创建独立 Prometheus registry，并注册 `process` 和 `go` collectors。

如果应用已有 registry，可以使用 `NewWithRegisterer`：

```go
registry := prometheus.NewRegistry()

provider, err := metrics.NewWithRegisterer(ctx, cfg, registry)
if err != nil {
	return err
}

http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
```

## 业务指标

业务代码应使用 OpenTelemetry Metrics API 创建指标，不要直接依赖 Prometheus client 作为主采集 API。

```go
meter := provider.MeterProvider().Meter("github.com/surge-go/fox/app")

requests, err := meter.Int64Counter("http_requests")
if err != nil {
	return err
}

requests.Add(ctx, 1)
```

这样同一套指标可以同时适配 OTLP、Prometheus 和 stdout 等不同出口。

## 关闭

应用退出时应调用 `Shutdown`：

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := provider.Shutdown(ctx); err != nil {
	return err
}
```

`Shutdown` 会关闭底层 `MeterProvider`，并在当前全局 provider 仍由本包持有时恢复为 noop provider。

## 默认值

| 配置 | 默认值 |
| --- | --- |
| `Exporter` | `none` |
| `Reader.Interval` | `15s` |
| `Reader.Timeout` | `5s` |
| `OTLP.Timeout` | `5s` |
| `OTLP.Compression` | `gzip` |
| `Service` | `nil` 时使用 OpenTelemetry 默认 resource |

如果配置了 `Service`，`Service.Name` 必填。

