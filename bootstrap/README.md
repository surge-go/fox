# bootstrap

`bootstrap` 是 fox 应用启动门面，用于按固定顺序初始化已有 `core` 模块，并统一管理应用生命周期。

它适合希望通过一份配置快速启动应用的场景；如果只需要某个单独模块，也可以继续直接使用对应的 `core/*` 包。

## 初始化顺序

`New` 会按以下顺序初始化模块：

1. `core/logger`
2. `core/tracing`
3. `core/metrics`
4. `core/database`
5. `core/redis`
6. `core/server`

任意步骤失败时，会逆序清理已经初始化的资源，并把初始化错误和清理错误通过 `errors.Join` 一起返回。

## 快速开始

```go
package main

import (
	"log"

	"github.com/surge-go/fox/bootstrap"
	"github.com/surge-go/fox/core/logger"
	"github.com/surge-go/fox/core/server"
)

func main() {
	app, err := bootstrap.New(&bootstrap.Config{
		Logger: &logger.Config{
			Level:  logger.LevelInfo,
			Output: logger.OutputStdout,
		},
		Server: &server.Config{
			Mode: server.ModeDebug,
			Addr: ":8080",
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	if srv := app.Server(); srv != nil {
		srv.GET("/health", func(c *server.Context) {
			c.Ok(map[string]string{"status": "ok"})
		})
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
```

## 从配置文件加载

`LoadConfig` 使用 `core/config` 读取配置文件，并解析为 `bootstrap.Config`。

```go
package main

import (
	"log"

	"github.com/surge-go/fox/bootstrap"
	"github.com/surge-go/fox/core/server"
)

func main() {
	cfg, err := bootstrap.LoadConfig("config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	app, err := bootstrap.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	if srv := app.Server(); srv != nil {
		srv.GET("/ready", func(c *server.Context) {
			c.Ok(map[string]bool{"ready": true})
		})
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
```

示例配置：

```yaml
logger:
  level: info
  output: stdout

server:
  mode: debug
  addr: ":8080"
  read_timeout: 5s
  write_timeout: 10s
```

也可以完全使用 `core/config.Option`：

```go
cfg, err := bootstrap.LoadConfigWithOptions(
	coreconfig.WithConfigName("app"),
	coreconfig.WithConfigType("yaml"),
	coreconfig.WithConfigPaths("./config"),
	coreconfig.WithEnvPrefix("FOX"),
)
```

`LoadConfig(path, opts...)` 会先使用 `path` 作为默认配置文件，再应用传入的 `opts`。如果 `opts` 中包含 `core/config.WithConfigFile`，它会覆盖默认路径。

## 生命周期钩子

`OnStart` 在 `server.Run` 前按注册顺序执行。任意启动钩子返回错误时，应用会执行清理并返回错误。

`OnStop` 在资源清理阶段按注册逆序执行。停止钩子返回的错误会被收集到最终关闭错误中。

```go
app.OnStart(func(ctx context.Context) error {
	app.Logger().Info("应用启动")
	return nil
})

app.OnStop(func(ctx context.Context) error {
	app.Logger().Info("应用停止")
	return nil
})
```

`nil` 钩子会被忽略。

## 关闭语义

`Run` 会执行启动钩子，然后启动 HTTP server。当前 HTTP server 的系统信号监听和优雅关闭由 `core/server` 的 `Run` 负责。

`Shutdown(ctx)` 可由调用方主动触发，关闭顺序为：

1. HTTP server
2. 停止钩子
3. Redis
4. Database
5. Metrics
6. Tracing
7. Logger

`Shutdown` 是幂等的，多次调用只会执行一次清理，并返回第一次关闭保存的错误。

## 访问器

`Application` 提供以下访问器：

| 方法 | 未配置时 |
|------|----------|
| `Logger()` | 返回 `zap.L()` |
| `Server()` | 返回 `nil` |
| `DB()` | 返回 `nil` |
| `Redis()` | 返回 `nil` |
| `Tracing()` | 返回 `nil` |
| `Metrics()` | 返回 `nil` |

## 注意事项

1. `bootstrap` 只负责编排已有 `core` 模块，不替代各模块自身的配置和能力。
2. `core/tracing` 和 `core/metrics` 是应用级 provider，重复初始化会返回 already initialized 错误。
3. `New` 会保存顶层 `Config` 的浅拷贝；各子配置仍按指针传给对应 `core` 模块初始化。
4. `LoadConfig` 只读取配置快照，不持有 `core/config` 的热更新生命周期。
