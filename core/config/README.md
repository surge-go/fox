# core/config

基于 Viper 的配置管理器，支持多格式配置文件、环境变量、默认值和热更新。

## 快速开始

```go
package main

import (
    "log"
    "github.com/surge-go/fox/core/config"
)

func main() {
    // 创建配置管理器
    cfg := config.New(
        config.WithFile("config.yaml"),
        config.WithEnvPrefix("APP"),
    )

    // 加载配置
    if err := cfg.Load(); err != nil {
        log.Fatal(err)
    }

    // 读取配置
    port := cfg.GetInt("server.port")
    dbHost := cfg.GetString("database.host")
}
```

## 核心功能

### 1. 多格式支持
支持 JSON、YAML、TOML、HCL、INI、ENV 等格式。

```go
cfg := config.New(
    config.WithFile("config.yaml"),  // 自动识别格式
)
```

### 2. 环境变量覆盖
```go
cfg := config.New(
    config.WithEnvPrefix("APP"),  // APP_SERVER_PORT 映射到 server.port
)
```

### 3. 默认值
```go
cfg := config.New(
    config.WithDefaults(map[string]any{
        "server.port": 8080,
        "server.host": "0.0.0.0",
    }),
)
```

### 4. 配置热更新
```go
cfg := config.New(
    config.WithFile("config.yaml"),
    config.WithAutoWatch(true),  // 自动监听文件变化
    config.WithOnChange(func() {
        log.Println("配置已更新")
    }),
)
```

### 5. 配置保护模式
防止配置文件被意外修改导致服务异常。

```go
cfg := config.New(
    config.WithFile("config.yaml"),
    config.WithProtected(true),  // 启用保护模式
)
```

## API 参考

### 创建配置管理器

```go
// 使用选项模式创建
cfg := config.New(
    config.WithFile("config.yaml"),
    config.WithEnvPrefix("APP"),
    config.WithDefaults(defaults),
)

// 获取全局默认实例
cfg := config.Default()

// 设置全局默认实例
config.SetDefault(cfg)
```

### 读取配置

```go
// 基础类型
cfg.GetString("key")
cfg.GetInt("key")
cfg.GetBool("key")
cfg.GetFloat64("key")
cfg.GetDuration("key")

// 复杂类型
cfg.GetStringSlice("key")
cfg.GetStringMap("key")
cfg.GetStringMapString("key")

// 反序列化到结构体
type ServerConfig struct {
    Port int    `mapstructure:"port"`
    Host string `mapstructure:"host"`
}
var server ServerConfig
cfg.Unmarshal("server", &server)
```

### 设置配置

```go
cfg.Set("key", "value")
cfg.SetDefault("key", "default")
```

### 配置监听

```go
// 手动启动监听
cfg.Watch()

// 停止监听
cfg.StopWatch()

// 设置回调
cfg.SetOnChange(func() {
    log.Println("配置已更新")
})
```

## 配置文件示例

### YAML
```yaml
server:
  port: 8080
  host: 0.0.0.0
  timeout: 30s

database:
  driver: mysql
  dsn: "user:pass@tcp(localhost:3306)/db"
  pool:
    max_open_conns: 100
    max_idle_conns: 10
```

### 环境变量
```bash
# 使用 WithEnvPrefix("APP") 时
export APP_SERVER_PORT=8080
export APP_DATABASE_DSN="user:pass@tcp(localhost:3306)/db"
```

## 最佳实践

### 1. 配置分层
```
config/
├── default.yaml      # 默认配置
├── development.yaml  # 开发环境
├── staging.yaml      # 预发环境
└── production.yaml   # 生产环境
```

```go
env := os.Getenv("ENV")
cfg := config.New(
    config.WithFile("config/default.yaml"),
    config.WithFile(fmt.Sprintf("config/%s.yaml", env)),
)
```

### 2. 配置验证
```go
type Config struct {
    Server   ServerConfig   `mapstructure:"server"`
    Database DatabaseConfig `mapstructure:"database"`
}

var appConfig Config
if err := cfg.UnmarshalExact(&appConfig); err != nil {
    log.Fatal("配置格式错误:", err)
}

// 自定义验证
if appConfig.Server.Port < 1024 {
    log.Fatal("端口号必须大于 1024")
}
```

### 3. 敏感信息处理
```go
// 从环境变量读取敏感信息
cfg := config.New(
    config.WithFile("config.yaml"),
    config.WithEnvPrefix("APP"),
)

// 配置文件中使用占位符
// database:
//   password: ${DB_PASSWORD}  # 从环境变量读取
```

### 4. 配置热更新
```go
cfg := config.New(
    config.WithFile("config.yaml"),
    config.WithAutoWatch(true),
    config.WithOnChange(func() {
        // 重新加载配置
        var newConfig Config
        cfg.Unmarshal(&newConfig)
        
        // 更新运行时配置
        updateRuntimeConfig(newConfig)
    }),
)
```

## 注意事项

1. **配置文件路径**：支持相对路径和绝对路径，相对路径基于工作目录
2. **环境变量优先级**：环境变量 > 配置文件 > 默认值
3. **热更新限制**：某些配置（如端口号）需要重启服务才能生效
4. **并发安全**：所有读写操作都是并发安全的
5. **保护模式**：启用后会自动恢复被修改的配置文件

## 依赖

- `github.com/spf13/viper` - 配置管理核心库
- `github.com/fsnotify/fsnotify` - 文件监听（热更新）

## 相关文档

- [Viper 官方文档](https://github.com/spf13/viper)
- [配置最佳实践](https://12factor.net/config)
