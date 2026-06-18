# Fox

Fox 是一个基于 Gin 的轻量 HTTP server 包。它保留 Gin 的高性能路由能力，同时在根包提供更稳定的工程入口：统一配置、路由分组、启动路由打印、请求上下文、统一响应、panic recovery 和优雅关机。

## 特性

- 轻量 Engine：封装 `gin.Engine` 与 `http.Server`，统一管理启动、关闭、TLS、h2c 和超时配置。
- 路由注册：支持常见 HTTP 方法、`Any`、路由分组和分组中间件。
- 路由打印：启动时按运行时函数名打印路由表，便于检查注册结果。
- 启动打印：debug/test 模式默认打印 FOX banner、运行信息和路由表；release 模式默认关闭。
- 请求日志：`middleware.Logger()` 输出统一请求日志，支持自定义 logger、formatter 和跳过路径。
- 统一响应：提供 `Context.Ok`、`Context.Fail`、`Response` 和可扩展错误工厂。
- 内置恢复：`fox.New()` 默认注册 panic recovery，避免单个请求异常逃逸到 `net/http`。
- 优雅关机：`Run()` 监听 `SIGINT`、`SIGTERM`，按 `ShutdownTimeout` 统一关闭。
- 标准兼容：`Engine` 实现 `http.Handler`，可直接用于 `httptest`、标准库 server 或其它组合场景。

## 安装

```bash
go get github.com/surge-go/fox
```

## 快速开始

```go
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/surge-go/fox"
	"github.com/surge-go/fox/middleware"
)

type AppController struct{}

func (ctl *AppController) Ping(c *fox.Context) {
	c.String(http.StatusOK, "pong")
}

func (ctl *AppController) Hello(c *fox.Context) {
	name := c.DefaultQuery("name", "fox")
	c.Ok(map[string]any{"message": "hello " + name})
}

func main() {
	printRoutes := true
	app := fox.New(&fox.Config{
		Mode:            fox.ModeDebug,
		Addr:            ":8080",
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    10 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		PrintRoutes:     &printRoutes,
	})

	app.Use(middleware.Logger())
	app.Use(func(c *fox.Context) {
		traceID := c.GetHeader("X-Trace-ID")
		if traceID == "" {
			traceID = time.Now().Format("20060102150405.000000000")
		}
		c.SetTraceID(traceID)
		c.SetHeader("X-Trace-ID", traceID)
		c.Next()
	})

	controller := &AppController{}
	app.GET("/ping", controller.Ping)

	api := app.Group("/api")
	api.GET("/hello", controller.Hello)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
```

运行仓库示例：

```bash
go run ./example
```

请求验证：

```bash
curl http://localhost:8080/ping
curl 'http://localhost:8080/api/hello?name=fox'
```

## 启动输出

`ModeDebug` 和 `ModeTest` 默认打印路由表，`ModeRelease` 默认关闭。也可以通过 `PrintRoutes` 显式控制。

```text
[Fox-debug] GET     /ping                          --> main.(*AppController).Ping
[Fox-debug] GET     /api/hello                     --> main.(*AppController).Hello
```

请求日志由 `middleware.Logger()` 输出：

```text
[FOX] 2026/06/16 - 23:14:36 | 200 | 464µs | 127.0.0.1 | GET "/ping" | 20260616231436.123456000
```

最后一段是 `Context.TraceID()`，只有设置了 trace id 时才会出现。

## 配置

```go
type Config struct {
	Mode               fox.Mode
	Addr               string
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	ShutdownTimeout    time.Duration
	MaxHeaderBytes     int
	MaxMultipartMemory int
	TLS                *fox.TLSConfig
	TrustedProxies     []string
	PrintRoutes        *bool
	UseH2C             bool
}
```

常用字段说明：

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `Mode` | `ModeRelease` | 运行模式：`debug`、`release`、`test` |
| `Addr` | `:8080` | 监听地址，支持 `":8080"`、`"127.0.0.1:8080"`、`"8080"` |
| `ShutdownTimeout` | `30s` | 收到退出信号后的优雅关机等待时间 |
| `PrintRoutes` | 按模式决定 | `nil` 时 debug/test 打印，release 不打印 |
| `TrustedProxies` | `nil` | 可信代理 IP 或 CIDR，影响 `ClientIP()` |
| `TLS` | `nil` | HTTPS 配置 |
| `UseH2C` | `false` | 是否启用明文 HTTP/2，不能与 TLS 同时开启 |

`TLS.Config` 的优先级最高；提供后会覆盖 `CertFile`、`KeyFile`、`MinVersion` 和 `CipherSuites`。

## 路由

Fox 支持在 `Engine` 和 `RouteGroup` 上注册路由：

```go
app.GET("/ping", ping)
app.POST("/users", createUser)
app.PUT("/users/:id", updateUser)
app.DELETE("/users/:id", deleteUser)
app.Any("/proxy/*path", proxy)
```

路由打印使用 handler 的运行时函数名。具名函数会显示更清晰：

```go
func ping(c *fox.Context) {
	c.String(http.StatusOK, "pong")
}

app.GET("/ping", ping)

api := app.Group("/api")
api.GET("/users", listUsers)
api.POST("/users", createUser)
```

匿名函数在 Go 中通常会显示为 `main.main.func1` 这类名称；生产项目如果需要更清晰的路由表，建议使用具名函数。

## 中间件

全局中间件通过 `Use` 注册，只影响后续注册的路由：

```go
app.Use(middleware.Logger())
app.Use(middleware.CORS())
```

分组中间件通过 `Group` 或 `RouteGroup.Use` 注册：

```go
auth := func(c *fox.Context) {
	if c.GetHeader("Authorization") == "" {
		c.Fail(c.Errors().ErrUnauthorized())
		return
	}
	c.Next()
}

api := app.Group("/api", auth)
api.GET("/profile", profile)
```

需要复用 Gin 生态中间件时，可以使用 `UseGin`：

```go
app.UseGin(ginMiddleware)
```

### Logger

```go
app.Use(middleware.Logger())
```

自定义输出：

```go
app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
	Logger: customLogger,
	SkipPaths: []string{
		"/health",
	},
}))
```

`Logger` 使用 `fox.Logger` 接口：

```go
type Logger interface {
	Printf(format string, args ...any)
}
```

### CORS

```go
app.Use(middleware.CORSWithConfig(middleware.CORSConfig{
	AllowOrigins:     []string{"https://example.com", "*.example.com"},
	AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
	AllowHeaders:     []string{"Authorization", "Content-Type"},
	AllowCredentials: true,
	MaxAge:           12 * time.Hour,
}))
```

### Recovery

`fox.New()` 已默认注册内置 recovery。通常不需要手动添加。

如果需要自定义 panic 日志和堆栈输出，可以使用 `middleware.RecoveryWithConfig`。注意不要重复注册多个 recovery，避免日志重复或行为不清晰。

## Context

`fox.Context` 封装 Gin context，并补充了标准库 context 兼容能力。

常用方法：

```go
c.Param("id")
c.Query("name")
c.DefaultQuery("name", "fox")
c.BindJSON(&req)
c.Ok(data)
c.Fail(err)
c.SetTraceID(traceID)
c.TraceID()
c.RawRequest()
c.RawWriter()
c.StdContext()
```

`Ok` 输出统一响应：

```json
{
  "code": 200,
  "data": {},
  "message": "success",
  "trace_id": "20260616231436.123456000"
}
```

`Fail` 会识别 `core/errors` 中的错误类型，并按其中的 HTTP 状态码、业务码和公开消息输出；普通错误会隐藏为 `500 internal server error`。

## 优雅关机

`Run()` 会监听 `SIGINT` 和 `SIGTERM`：

```go
app := fox.New(&fox.Config{
	Addr:            ":8080",
	ShutdownTimeout: 15 * time.Second,
})

if err := app.Run(); err != nil {
	log.Fatal(err)
}
```

需要主动关闭时，可以直接调用：

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

if err := app.Shutdown(ctx); err != nil {
	log.Fatal(err)
}
```

## 测试

`Engine` 实现了 `http.Handler`，可以直接配合 `httptest`：

```go
app := fox.New(&fox.Config{
	Addr: ":0",
	Mode: fox.ModeTest,
})
app.GET("/ping", func(c *fox.Context) {
	c.String(http.StatusOK, "pong")
})

req := httptest.NewRequest(http.MethodGet, "/ping", nil)
rec := httptest.NewRecorder()
app.ServeHTTP(rec, req)
```

运行测试：

```bash
go test ./...
go vet ./...
```

## 目录结构

```text
fox/
├── config.go          # HTTP server 配置和校验
├── context.go         # 请求上下文封装
├── engine.go          # Engine 生命周期、启动、关闭、路由注册
├── error.go           # 错误接口定义
├── logger.go          # 启动 banner 和路由表打印
├── recovery.go        # New() 内置 panic recovery
├── router.go          # RouteGroup 和路径拼接
├── types.go           # HandlerFunc、Response、Logger、默认错误工厂
├── middleware/        # 可选中间件：Logger、CORS、Recovery
├── example/           # Fox HTTP 示例
├── core/              # 独立核心模块
└── pkg/               # 通用工具包
```

## 相关文档

- [example](./example/README.md)
- [middleware](./middleware/README.md)
- [core/config](./core/config/README.md)
- [core/database](./core/database/README.md)
- [core/logger](./core/logger/README.md)
- [core/metrics](./core/metrics/README.md)
- [core/redis](./core/redis/README.md)
- [core/tracing](./core/tracing/README.md)
- [pkg/file](./pkg/file/README.md)
