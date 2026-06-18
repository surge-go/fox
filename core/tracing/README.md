# Fox Tracing

`core/tracing` 负责初始化应用级 OpenTelemetry Tracing 管线，为数据库、Redis 和业务代码中的 span 提供统一出口。

该包只负责：

- 创建并设置全局 OpenTelemetry `TracerProvider`。
- 根据配置创建 trace exporter、sampler、resource 和 batch processor。
- 托管 `TracerProvider` 生命周期，提供 `Shutdown`。
- 提供 `GlobalTracerProvider` 便于框架内部或应用层读取当前 provider。

该包不负责自动注册 HTTP tracing 中间件，也不直接绑定具体 HTTP 路由实现。

## 快速开始

```go
package main

import (
	"context"
	"log"

	"github.com/surge-go/fox/core/tracing"
)

func main() {
	ctx := context.Background()

	provider, err := tracing.New(ctx, &tracing.Config{
		Service: &tracing.ServiceConfig{
			Name:        "fox-api",
			Namespace:   "surge",
			Version:     "v1.0.0",
			Environment: "prod",
		},
		Exporter: tracing.ExporterOTLPGRPC,
		OTLP: &tracing.OTLPConfig{
			Endpoint: "otel-collector:4317",
			Insecure: true,
		},
		Sampler: &tracing.SamplerConfig{
			Type:  tracing.SamplerParentBasedTraceIDRatio,
			Ratio: 0.1,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer provider.Shutdown(ctx)

	// 在这里初始化 database / redis / server / 业务模块。
}
```

`tracing.New` 会把创建出的 `TracerProvider` 设置为 OpenTelemetry 全局 provider。依赖全局 provider 的 instrumentation 应在它之后初始化。

## 初始化顺序

`core/database` 和 `core/redis` 的 `TracingEnabled` 只表示注册采集点，不表示 trace 已经能被导出。

推荐顺序：

```go
traceProvider, err := tracing.New(ctx, tracingCfg)
if err != nil {
	return err
}
defer traceProvider.Shutdown(ctx)

db, err := database.NewClient(databaseCfgWithTracingEnabled)
if err != nil {
	return err
}

redisClient, err := redis.NewClient(redisCfgWithTracingEnabled)
if err != nil {
	return err
}
```

如果没有先初始化 `core/tracing`，即使 database / redis 的 `TracingEnabled` 设置为 `true`，span 也可能只注册到默认 noop provider 上，无法导出。

## 单例行为

`core/tracing` 是应用级单例。第一次 `tracing.New` 成功后会占用全局 provider；再次调用 `tracing.New` 会返回 `tracing provider already initialized`。

需要重新初始化时，应先关闭当前 provider：

```go
if err := provider.Shutdown(ctx); err != nil {
	return err
}

provider, err = tracing.New(ctx, newCfg)
```

## Exporter

支持的导出方式：

| Exporter | 用途 |
| --- | --- |
| `none` | 创建可用的本地 `TracerProvider`，但不导出 span |
| `stdout` | 输出到 stdout，适合本地调试 |
| `otlp_grpc` | 通过 OTLP gRPC 上报到 OpenTelemetry Collector |
| `otlp_http` | 通过 OTLP HTTP/protobuf 上报到 Collector 或后端 |

默认 `Exporter` 为空时等同于 `none`。

## OTLP gRPC

```go
provider, err := tracing.New(ctx, &tracing.Config{
	Service: &tracing.ServiceConfig{
		Name: "fox-api",
	},
	Exporter: tracing.ExporterOTLPGRPC,
	OTLP: &tracing.OTLPConfig{
		Endpoint:    "otel-collector:4317",
		Insecure:    true,
		Timeout:     5 * time.Second,
		Compression: tracing.CompressionGzip,
	},
})
```

`otlp_grpc` 的 `Endpoint` 应使用 `host:port`，不要包含 `http://` 或 `https://`。

## OTLP HTTP

```go
provider, err := tracing.New(ctx, &tracing.Config{
	Service: &tracing.ServiceConfig{
		Name: "fox-api",
	},
	Exporter: tracing.ExporterOTLPHTTP,
	OTLP: &tracing.OTLPConfig{
		Endpoint: "http://otel-collector:4318",
		Insecure: true,
		Headers: map[string]string{
			"x-tenant-id": "fox",
		},
	},
})
```

`otlp_http` 的 `Endpoint` 可以使用完整 URL，也可以使用 `host:port` 并配合 `Insecure` 控制协议。

## Sampler

支持的采样策略：

| Sampler | 行为 |
| --- | --- |
| `always_on` | 所有 trace 都采样 |
| `always_off` | 所有 trace 都不采样 |
| `trace_id_ratio` | 按 trace id 比例采样 |
| `parent_based_always_on` | 尊重父 span，没有父 span 时默认采样 |
| `parent_based_trace_id_ratio` | 尊重父 span，没有父 span 时按比例采样 |

生产环境通常建议使用 `parent_based_trace_id_ratio`：

```go
Sampler: &tracing.SamplerConfig{
	Type:  tracing.SamplerParentBasedTraceIDRatio,
	Ratio: 0.1,
},
```

`Ratio` 合法范围为 `0` 到 `1`，例如 `0.1` 表示采样 10% 的新根 trace。

## Batch

OTLP 和 stdout exporter 默认使用 OpenTelemetry SDK 的 batch processor。需要调整批处理参数时可以配置 `Batch`：

```go
Batch: &tracing.BatchConfig{
	MaxQueueSize:       2048,
	BatchTimeout:       5 * time.Second,
	ExportTimeout:      30 * time.Second,
	MaxExportBatchSize: 512,
},
```

一般业务不需要显式配置 `Batch`，除非有明确的吞吐、延迟或内存控制目标。

## 业务 Span

业务代码应使用 OpenTelemetry Trace API 创建 span：

```go
tracer := provider.Tracer("github.com/surge-go/fox/app")

ctx, span := tracer.Start(ctx, "create_user")
defer span.End()

span.SetAttributes(attribute.String("user.email_domain", "example.com"))
```

如果不方便持有 `provider`，也可以通过 OpenTelemetry 全局 API 获取 tracer：

```go
tracer := otel.Tracer("github.com/surge-go/fox/app")
```

## 关闭

应用退出时应调用 `Shutdown`：

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := provider.Shutdown(ctx); err != nil {
	return err
}
```

`Shutdown` 会关闭底层 `TracerProvider`，并在当前全局 provider 仍由本包持有时恢复为 noop provider。

## 默认值

| 配置 | 默认值 |
| --- | --- |
| `Exporter` | `none` |
| `Sampler.Type` | `parent_based_always_on` |
| `OTLP.Timeout` | exporter 默认值 |
| `OTLP.Compression` | exporter 默认值 |
| `Batch` | OpenTelemetry SDK 默认值 |
| `Service` | `nil` 时使用 OpenTelemetry 默认 resource |

如果配置了 `Service`，`Service.Name` 必填。
