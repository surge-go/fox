# Fox

Fox 是一款面向快速开发的 Go Web 单体框架，基于 Gin 构建，提供开箱即用的企业级能力。通过 `bootstrap` 门面统一编排配置、日志、链路追踪、指标、数据库、Redis、HTTP Server 等模块，让开发者专注于业务逻辑。

## 特性

- **统一启动门面** — `bootstrap.Application` 按序初始化所有模块，失败时逆序清理，支持生命周期钩子
- **类型安全路由** — 泛型封装 10 种请求/响应变体（JSON、Query、URI、Form 等），编译期保证类型正确
- **可观测性** — OpenTelemetry 全链路集成：Tracing（OTLP gRPC/HTTP）、Metrics（OTLP/Prometheus）、结构化日志（Zap）
- **多数据库支持** — GORM 封装 MySQL/PostgreSQL/SQLite/SQL Server，支持读写分离、连接池、慢查询日志
- **Redis 高可用** — go-redis/v9 支持 Standalone/Sentinel/Cluster 模式，内置重试、TLS、链路追踪
- **配置热重载** — Viper 多格式（YAML/JSON/TOML）、环境变量覆盖、文件监听自动重载
- **内置中间件** — Recovery、请求日志、限流器、CORS、Tracing、Metrics，一行代码启用
- **OpenAPI 文档** — 从路由自动生成 OpenAPI 3.0.3 文档，内置 UI 和请求调试代理
- **统一错误模型** — 业务错误码 + HTTP 状态码 + 错误链，兼容 `errors.Is`/`errors.As`

## 安装

```bash
go get github.com/surge-go/fox@v0.0.1
```

按需引入子模块：

```bash
# 仅使用 HTTP 服务器
go get github.com/surge-go/fox/core/server

# 仅使用数据库客户端
go get github.com/surge-go/fox/core/database

# 仅使用文件工具包
go get github.com/surge-go/fox/pkg/file
```

要求 Go 1.21+。

## 快速开始

```go
package main

import (
	"context"
	"log"

	"github.com/surge-go/fox/bootstrap"
	"github.com/surge-go/fox/core/server"
)

func main() {
	app, err := bootstrap.New(&bootstrap.Config{
		Server: &server.Config{
			Mode: server.ModeDebug,
			Addr: ":8080",
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	// 注册路由
	app.Server().GET("/ping", func(c *server.Context) {
		c.JSON(200, server.H{"message": "pong"})
	})

	// 启动应用（阻塞，监听信号后优雅关闭）
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
```

## 从配置文件启动

```go
package main

import (
	"log"

	"github.com/surge-go/fox/bootstrap"
)

func main() {
	app, err := bootstrap.LoadConfig("config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	app.Server().GET("/ping", func(c *server.Context) {
		c.JSON(200, server.H{"message": "pong"})
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
```

对应的 `config.yaml`：

```yaml
server:
  mode: debug
  addr: ":8080"

logger:
  level: info
  output: stdout

database:
  driver: mysql
  dsn: "user:pass@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=True"

redis:
  addr: "127.0.0.1:6379"
  db: 0

tracing:
  endpoint: "localhost:4317"
  protocol: grpc

metrics:
  endpoint: "localhost:4317"
  protocol: grpc
```

## 项目结构

```
fox/
├── bootstrap/          # 应用启动门面，统一编排各模块初始化和生命周期
│   ├── application.go  # Application 核心：New、Run、Shutdown、生命周期钩子
│   ├── config.go       # 顶层 Config 聚合
│   └── load.go         # 从文件加载配置并创建 Application
│
├── core/               # 核心模块，每个包独立可用
│   ├── config/         # 配置管理（Viper 封装，多格式、环境变量、热重载）
│   ├── database/       # 数据库客户端（GORM，MySQL/PG/SQLite/MSSQL，读写分离）
│   ├── errors/         # 统一错误模型（业务码、HTTP 状态码、错误链）
│   ├── logger/         # 结构化日志（Zap，JSON/Console，文件轮转，采样）
│   ├── metrics/        # 指标采集（OpenTelemetry Metrics，OTLP/Prometheus）
│   ├── redis/          # Redis 客户端（Standalone/Sentinel/Cluster，TLS，重试）
│   ├── server/         # HTTP 服务器（Gin 封装，泛型路由，中间件，优雅关闭）
│   └── tracing/        # 链路追踪（OpenTelemetry Tracing，OTLP gRPC/HTTP）
│
├── pkg/                # 通用工具包
│   ├── file/           # 文件与目录管理（读写、复制、遍历、Glob、监听、锁、临时文件）
│   └── openapi/        # OpenAPI 3.0.3 文档生成与 UI
│
└── internal/           # 内部包（不对外暴露）
```

## 核心模块

### bootstrap — 应用门面

统一编排所有模块的初始化和关闭，按固定顺序启动：Logger → Tracing → Metrics → Database → Redis → Server。任意步骤失败时逆序关闭已初始化的模块。

```go
app, _ := bootstrap.New(&bootstrap.Config{
    Logger:  &logger.Config{Level: "info", Output: logger.OutputStdout},
    Server:  &server.Config{Mode: server.ModeDebug, Addr: ":8080"},
    Database: &database.Config{Driver: "mysql", DSN: "..."},
    Redis:   &redis.Config{Addr: "127.0.0.1:6379"},
    Tracing: &tracing.Config{Endpoint: "localhost:4317", Protocol: "grpc"},
    Metrics: &metrics.Config{Endpoint: "localhost:4317", Protocol: "grpc"},
})

// 生命周期钩子
app.OnStart(func(ctx context.Context) error {
    // 初始化完成后执行
    return nil
})
app.OnStop(func(ctx context.Context) error {
    // 关闭前执行
    return nil
})

app.Run()
```

### server — HTTP 服务器

基于 Gin 的企业级 HTTP 服务器，提供泛型类型安全路由、内置中间件、优雅关闭。

```go
// 类型安全路由（10 种变体）
app.Server().POST("/users", createUser)
app.Server().GET("/users/:id", getUser)

func createUser(c *server.Context) {
    var req CreateUserRequest
    if err := c.BindJSON(&req); err != nil {
        return
    }
    // 业务逻辑...
    c.JSON(200, server.H{"id": user.ID})
}

func getUser(c *server.Context) {
    id, _ := c.ParamInt64("id")
    // 业务逻辑...
    c.JSON(200, user)
}
```

内置中间件：

```go
app.Server().Use(
    middleware.Recovery(),      // panic 恢复
    middleware.Logger(),        // 请求日志
    middleware.RateLimiter(100), // 限流（100 QPS）
    middleware.CORS(),          // 跨域
    middleware.Tracing(),       // 链路追踪
    middleware.Metrics(),       // 指标采集
)
```

### database — 数据库客户端

GORM 封装，支持多驱动、读写分离、链路追踪。

```go
db, _ := database.NewClient(&database.Config{
    Driver: "mysql",
    DSN:    "user:pass@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=True",
})

// 带日志的客户端
db, _ = database.NewClientWithLogger(cfg, app.Logger())
```

### redis — Redis 客户端

go-redis/v9 封装，支持多种部署模式。

```go
rdb, _ := redis.NewClient(&redis.Config{
    Addr:     "127.0.0.1:6379",
    Password: "",
    DB:       0,
})

// Sentinel 模式
rdb, _ = redis.NewClient(&redis.Config{
    Addrs:    []string{"sentinel-1:26379", "sentinel-2:26379"},
    Master:   "mymaster",
    Password: "secret",
})
```

### logger — 结构化日志

基于 Zap 的高性能日志，支持 JSON/Console 格式、文件轮转、采样。

```go
l, _ := logger.New(&logger.Config{
    Level:  "info",
    Format: "json",
    Output: logger.OutputFile,
    File:   "logs/app.log",
})

l.Info("server started", zap.String("addr", ":8080"))
```

### tracing — 链路追踪

OpenTelemetry Tracing，支持 OTLP gRPC/HTTP 导出。

```go
tp, _ := tracing.New(context.Background(), &tracing.Config{
    Endpoint: "localhost:4317",
    Protocol: "grpc",
    Sampling: 1.0, // 采样率
})
defer tp.Shutdown(context.Background())
```

### metrics — 指标采集

OpenTelemetry Metrics，支持 OTLP 和 Prometheus 导出。

```go
mp, _ := metrics.New(context.Background(), &metrics.Config{
    Endpoint: "localhost:4317",
    Protocol: "grpc",
})
defer mp.Shutdown(context.Background())
```

### config — 配置管理

Viper 封装，支持多格式文件、环境变量、默认值、热重载。

```go
cfg, _ := config.New(&config.Config{
    Path:      "config.yaml",
    FileType:  "yaml",
    EnvPrefix: "APP",
})
```

### errors — 统一错误模型

业务错误码 + HTTP 状态码 + 错误链。

```go
// 定义业务错误
var ErrUserNotFound = errors.New(10001, "user not found").WithStatus(404)

// 错误链
err := errors.New(10001, "user not found").
    WithErr(dbErr).
    WithStatus(404).
    WithMessage("用户不存在")

// 判断错误类型
if errors.Is(err, ErrUserNotFound) { ... }
```

## 工具包

### pkg/file — 文件管理

全面的文件系统操作 API：读写、复制、遍历、Glob、监听、锁、临时文件。

```go
file.WriteString("/tmp/hello.txt", "你好")
data, _ := file.ReadString("/tmp/hello.txt")

results, _ := file.Glob("/project", "**/*.go")
```

### pkg/openapi — OpenAPI 文档

从 Go 结构体自动生成 OpenAPI 3.0.3 文档，内置 UI 和请求调试代理。

```go
doc := openapi.New("My API", "1.0.0")
doc.AddSchema(CreateUserRequest{})
```

## 依赖

| 依赖 | 用途 |
|------|------|
| `github.com/gin-gonic/gin` | HTTP 路由和中间件 |
| `gorm.io/gorm` | ORM 数据库操作 |
| `github.com/redis/go-redis/v9` | Redis 客户端 |
| `github.com/spf13/viper` | 配置管理 |
| `go.uber.org/zap` | 高性能结构化日志 |
| `go.opentelemetry.io/otel` | 链路追踪和指标 |
| `github.com/fsnotify/fsnotify` | 文件系统事件通知 |
| `github.com/prometheus/client_golang` | Prometheus 指标导出 |

## 相关文档

- [bootstrap](./bootstrap/README.md) — 应用门面
- [core/server](./core/server/README.md) — HTTP 服务器
- [core/config](./core/config/README.md) — 配置管理
- [core/database](./core/database/README.md) — 数据库客户端
- [core/redis](./core/redis/README.md) — Redis 客户端
- [core/logger](./core/logger/README.md) — 结构化日志
- [core/tracing](./core/tracing/README.md) — 链路追踪
- [core/metrics](./core/metrics/README.md) — 指标采集
- [core/errors](./core/errors/README.md) — 统一错误模型
- [pkg/file](./pkg/file/README.md) — 文件管理
- [pkg/openapi](./pkg/openapi/README.md) — OpenAPI 文档
