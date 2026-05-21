# Fox Server

企业级 HTTP 服务器框架，基于 Gin 封装，提供类型安全、开箱即用的 Web 服务能力。

## 核心特性

### 🎯 泛型包装器 - 零样板代码
- **自动参数绑定**：JSON/Query/URI 参数自动解析和验证
- **统一响应格式**：自动包装为 `Response{code, message, data}` 结构
- **编译期类型安全**：泛型保证请求/响应类型正确
- **代码减少 67%**：从 15 行样板代码降至 5 行业务逻辑

### 🚀 生产就绪
- **优雅关闭**：自动处理 SIGTERM/SIGINT 信号，等待请求完成
- **TLS/H2C 支持**：开箱即用的 HTTPS 和 HTTP/2 Cleartext
- **配置校验**：启动时自动验证配置合法性
- **敏感信息脱敏**：日志中自动隐藏密码、Token 等敏感字段

### 🛡️ 企业级中间件
- **Recovery**：自动捕获 panic 并返回 500 错误（已自动注册）
- **Logger**：结构化日志，按模式和配置自动启用
- **Tracing**：当全局 OpenTelemetry provider 已初始化时自动接入
- **RateLimiter**：令牌桶算法限流，支持突发流量和自定义限流键

### 📦 渐进式增强
- 新旧代码可共存，无需一次性重构
- 零性能损耗（泛型编译期展开）

---

## 快速开始

### 安装

```bash
go get github.com/surge-go/fox/core/server
```

### 最小示例

```go
package main

import (
    "github.com/surge-go/fox/core/server"
)

func main() {
    // 1. 创建服务器
    srv, _ := server.New(&server.Config{
        Addr: ":8080",
        Mode: server.ModeRelease,
    })

    // 2. 注册路由（使用泛型包装器）
    srv.POST("/users", server.BindJSON(CreateUser))

    // 3. 启动服务（自动优雅关闭）
    srv.Run()
}

// 业务逻辑：只需关注输入输出
func CreateUser(c *server.Context, req *CreateUserReq) (*CreateUserResp, error) {
    // 参数已自动绑定和验证
    user := &User{Name: req.Name, Email: req.Email}
    
    // 返回数据会自动包装为 Response{code:200, data:user}
    return &CreateUserResp{ID: 1, Name: user.Name}, nil
}

type CreateUserReq struct {
    Name  string `json:"name" binding:"required"`
    Email string `json:"email" binding:"required,email"`
}

type CreateUserResp struct {
    ID   int64  `json:"id"`
    Name string `json:"name"`
}
```

**响应示例**：
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "id": 1,
    "name": "张三"
  }
}
```

---

## 核心概念

### 1. 配置（Config）

```go
cfg := &server.Config{
    // 基础配置
    Addr: ":8080",              // 监听地址
    Mode: server.ModeRelease,   // 运行模式：debug/release/test
    EnableLogger: nil,          // nil 表示非 release 默认启用，release 默认关闭
    
    // 超时配置
    ReadTimeout:  30 * time.Second,
    WriteTimeout: 30 * time.Second,
    
    // 安全配置
    TrustedProxies: []string{"127.0.0.1"},
    
    // TLS 配置（可选）
    TLS: &server.TLSConfig{
        CertFile: "cert.pem",
        KeyFile:  "key.pem",
    },
    
    // HTTP/2 Cleartext（可选）
    UseH2C: true,
}

srv, err := server.New(cfg)
```

#### 内置中间件行为

- `Recovery`：始终自动注册
- `Logger`：默认在 `debug` / `test` 模式启用，`release` 模式默认关闭
- `Tracing`：如果你先通过 `core/tracing.New` 初始化了全局 provider，`server.New` 会自动注册；provider `Shutdown` 后会自动清理全局状态
- `RateLimiter`：仍然保留手动注册，适合按路由组或业务域配置

### 2. 泛型包装器（Wrapper）

#### 核心签名
```go
type BizHandler[Req, Resp any] func(c *Context, req *Req) (*Resp, error)
```

#### 10 种包装器

| 包装器 | 绑定方式 | 使用场景 | 示例 |
|--------|---------|---------|------|
| `BindJSON` | JSON Body | POST/PUT 请求体 | 创建/更新资源 |
| `BindQuery` | URL Query | GET 请求参数 | 列表查询、筛选 |
| `BindURI` | URL Path | RESTful 路径参数 | `/users/:id` |
| `Bind` | 自动检测 | 混合场景 | 同时支持 JSON/Form |
| `NoReq` | 无请求参数 | 简单查询 | 健康检查、统计 |
| `NoResp` | 无响应数据 | 删除操作 | 返回 `{code:200}` |
| `NoRespJSON` | JSON + 无响应 | 删除+JSON参数 | 批量删除 |
| `NoRespQuery` | Query + 无响应 | 删除+Query参数 | 条件删除 |
| `NoRespURI` | URI + 无响应 | 删除+路径参数 | 按 ID 删除 |
| `Simple` | 无请求无响应 | 最简场景 | Ping/Pong |

#### 完整示例

```go
// 1. BindJSON - 创建用户
srv.POST("/users", server.BindJSON(func(c *server.Context, req *CreateUserReq) (*User, error) {
    user := &User{Name: req.Name}
    // 自动返回: {"code":200, "data":{"id":1,"name":"张三"}}
    return user, nil
}))

// 2. BindQuery - 用户列表（分页）
srv.GET("/users", server.BindQuery(func(c *server.Context, req *ListUsersReq) (*ListUsersResp, error) {
    users := []User{{ID: 1, Name: "张三"}}
    return &ListUsersResp{Users: users, Total: 1}, nil
}))

type ListUsersReq struct {
    Page     int `form:"page" binding:"required,min=1"`
    PageSize int `form:"page_size" binding:"required,min=1,max=100"`
}

// 3. BindURI - 用户详情
srv.GET("/users/:id", server.BindURI(func(c *server.Context, req *GetUserReq) (*User, error) {
    user := findUserByID(req.ID)
    return user, nil
}))

type GetUserReq struct {
    ID int64 `uri:"id" binding:"required,min=1"`
}

// 4. NoRespURI - 删除用户
srv.DELETE("/users/:id", server.NoRespURI(func(c *server.Context, req *DeleteUserReq) error {
    deleteUser(req.ID)
    // 自动返回: {"code":200, "message":"success"}
    return nil
}))

// 5. NoReq - 健康检查
srv.GET("/health", server.NoReq(func(c *server.Context) (*HealthResp, error) {
    return &HealthResp{Status: "ok"}, nil
}))

// 6. Simple - Ping
srv.GET("/ping", server.Simple(func(c *server.Context) error {
    // 自动返回: {"code":200, "message":"success"}
    return nil
}))
```

### 3. 错误处理

#### 业务错误（自动识别）
```go
import "github.com/surge-go/fox/core/errors"

func CreateUser(c *server.Context, req *CreateUserReq) (*User, error) {
    if userExists(req.Email) {
        // 返回业务错误，自动转换为 {"code":400, "message":"邮箱已存在"}
        return nil, errors.New(400, "邮箱已存在")
    }
    return &User{ID: 1}, nil
}
```

#### 系统错误（自动隐藏）
```go
func GetUser(c *server.Context, req *GetUserReq) (*User, error) {
    user, err := db.FindUser(req.ID)
    if err != nil {
        // 系统错误自动返回 {"code":500, "message":"internal server error"}
        // 真实错误记录到日志，不暴露给客户端
        return nil, err
    }
    return user, nil
}
```

#### 手动控制响应
```go
srv.GET("/custom", func(c *server.Context) {
    if unauthorized() {
        c.Fail(errors.New(401, "未授权"))
        return
    }
    c.Ok(map[string]string{"status": "ok"})
})
```

### 4. 中间件

#### 全局中间件
```go
import "github.com/surge-go/fox/core/server/middleware"

// Recovery 已自动注册，无需手动添加
// Logger 已内置，非 release 模式默认启用；可通过 Config.EnableLogger 显式控制
// Tracing 会在 core/tracing.New 初始化全局 provider 后自动接入，provider 关闭时自动清理
// 如需手动指定 provider，仍可继续使用 middleware.Tracing(tracerProvider)

// 添加限流中间件（每秒 100 个请求，突发容量 200）
srv.Use(middleware.RateLimiter(&middleware.RateLimiterConfig{
    RequestsPerSecond: 100,
    Burst:             200,
}))

// 自定义中间件
srv.Use(AuthMiddleware())
```

#### 路由组中间件
```go
api := srv.Group("/api/v1")

// 为该路由组添加限流（每秒 10 个请求）
api.Use(middleware.RateLimiter(&middleware.RateLimiterConfig{
    RequestsPerSecond: 10,
    Burst:             20,
}))

api.POST("/users", server.BindJSON(CreateUser))
```

#### 自动启用 Tracing
```go
import (
    "context"

    "github.com/surge-go/fox/core/server"
    "github.com/surge-go/fox/core/tracing"
)

provider, err := tracing.New(context.Background(), &tracing.Config{
    Exporter: tracing.ExporterStdout,
})
if err != nil {
    panic(err)
}
defer provider.Shutdown(context.Background())

srv, _ := server.New(&server.Config{
    Addr: ":8080",
    Mode: server.ModeDebug,
})
// 这里不需要再手动 srv.Use(Tracing(...))
```

#### 自定义中间件
```go
func AuthMiddleware() server.HandlerFunc {
    return func(c *server.Context) {
        token := c.GetHeader("Authorization")
        if !validateToken(token) {
            c.Fail(errors.New(401, "未授权"))
            return
        }
        c.Next()
    }
}
```

### 5. 路由组

```go
// API v1
v1 := srv.Group("/api/v1")
v1.Use(AuthMiddleware())
{
    v1.POST("/users", server.BindJSON(CreateUser))
    v1.GET("/users/:id", server.BindURI(GetUser))
}

// API v2
v2 := srv.Group("/api/v2")
v2.Use(NewAuthMiddleware())
{
    v2.POST("/users", server.BindJSON(CreateUserV2))
}
```

---

## 高级特性

### 1. TLS/HTTPS

```go
srv, _ := server.New(&server.Config{
    Addr: ":443",
    TLS: &server.TLSConfig{
        CertFile: "/path/to/cert.pem",
        KeyFile:  "/path/to/key.pem",
        // 可选：自定义 TLS 配置
        Config: &tls.Config{
            MinVersion: tls.VersionTLS12,
        },
    },
})
```

### 2. HTTP/2 Cleartext (H2C)

```go
srv, _ := server.New(&server.Config{
    Addr:   ":8080",
    UseH2C: true, // 启用 HTTP/2 without TLS
})
```

### 3. 优雅关闭

```go
srv, _ := server.New(&server.Config{
    Addr: ":8080",
})

// Run() 会自动监听 SIGTERM/SIGINT 信号
// 收到信号后等待最多 30 秒让请求完成
srv.Run() // 阻塞直到收到信号
```

### 4. 文件上传

```go
srv.POST("/upload", func(c *server.Context) {
    file, _ := c.FormFile("file")
    c.SaveUploadedFile(file, "/tmp/"+file.Filename)
    c.Ok(map[string]string{"filename": file.Filename})
})
```

### 5. 静态文件

```go
srv.Static("/static", "./public")
srv.StaticFile("/favicon.ico", "./public/favicon.ico")
```

---

## 性能数据

### 泛型包装器性能
- **延迟**：2467ns vs 2473ns（传统方式），差异 0.24%
- **内存分配**：41 次/请求，与传统方式完全一致
- **结论**：零性能损耗（泛型编译期展开）

### 代码效率提升
| 指标 | 传统方式 | 泛型包装器 | 提升 |
|------|---------|-----------|------|
| 代码行数 | 15 行 | 5 行 | **67% ↓** |
| 样板代码 | 10 行 | 0 行 | **100% ↓** |
| 类型安全 | 运行时 | 编译期 | **✓** |
| 错误处理 | 手动 | 自动 | **✓** |

---

## 最佳实践

### 1. 请求结构体设计

```go
type CreateUserReq struct {
    // 使用 binding 标签进行验证
    Name  string `json:"name" binding:"required,min=2,max=50"`
    Email string `json:"email" binding:"required,email"`
    Age   int    `json:"age" binding:"omitempty,min=1,max=150"`
    
    // 使用 json:"-" 忽略字段
    Internal string `json:"-"`
}
```

**常用验证规则**：
- `required`：必填
- `min/max`：最小/最大值（数字）或长度（字符串）
- `email`：邮箱格式
- `url`：URL 格式
- `oneof=red green`：枚举值
- `omitempty`：可选字段

### 2. 响应结构体设计

```go
type ListUsersResp struct {
    Users []User `json:"users"`
    Total int64  `json:"total"`
    Page  int    `json:"page"`
}

// 嵌套结构
type User struct {
    ID        int64     `json:"id"`
    Name      string    `json:"name"`
    Email     string    `json:"email"`
    CreatedAt time.Time `json:"created_at"`
}
```

### 3. 错误处理策略

```go
func CreateUser(c *server.Context, req *CreateUserReq) (*User, error) {
    // 1. 参数验证错误（400）
    if req.Age < 18 {
        return nil, errors.New(400, "年龄必须大于18岁")
    }
    
    // 2. 业务逻辑错误（400/409）
    if userExists(req.Email) {
        return nil, errors.New(409, "邮箱已存在")
    }
    
    // 3. 系统错误（500，自动隐藏）
    user, err := db.Create(req)
    if err != nil {
        return nil, err // 自动返回 500，日志记录真实错误
    }
    
    return user, nil
}
```

### 4. 中间件顺序

```go
import "github.com/surge-go/fox/core/server/middleware"

// 内置顺序：
// 1. Recovery
// 2. Tracing（如果全局 provider 已初始化）
// 3. Logger（按 Mode / Config.EnableLogger 决定）
// 4. 你的自定义中间件

srv.Use(middleware.RateLimiter(&middleware.RateLimiterConfig{
    RequestsPerSecond: 100,
    Burst:             200,
}))

// 用户自定义中间件（示例）
srv.Use(CORSMiddleware())       // CORS（需自行实现）
srv.Use(AuthMiddleware())       // 认证（需自行实现）
```

#### 限流中间件配置

**全局限流**（所有请求共享一个桶）：
```go
srv.Use(middleware.RateLimiter(&middleware.RateLimiterConfig{
    RequestsPerSecond: 100,
    Burst:             200,
    KeyFunc: func(c *server.Context) string {
        return "global" // 固定键，所有请求共享
    },
}))
```

**按 IP 限流**（默认，每个 IP 独立限流）：
```go
srv.Use(middleware.RateLimiter(&middleware.RateLimiterConfig{
    RequestsPerSecond: 10,
    Burst:             20,
    // KeyFunc 默认使用 c.ClientIP()
}))
```

**按用户 ID 限流**：
```go
srv.Use(middleware.RateLimiter(&middleware.RateLimiterConfig{
    RequestsPerSecond: 50,
    Burst:             100,
    KeyFunc: func(c *server.Context) string {
        // 从上下文获取用户 ID
        return c.GetString("user_id")
    },
}))
```

**自定义限流响应**：
```go
srv.Use(middleware.RateLimiter(&middleware.RateLimiterConfig{
    RequestsPerSecond: 100,
    Burst:             200,
    OnLimitExceeded: func(c *server.Context) error {
        // 返回自定义错误
        return errors.NewWithStatus(4290, 429, "请求过于频繁，请稍后再试")
    },
}))
```

**路由组限流**：
```go
// 为特定路由组设置不同的限流策略
api := srv.Group("/api/v1")
api.Use(middleware.RateLimiter(&middleware.RateLimiterConfig{
    RequestsPerSecond: 10,
    Burst:             20,
}))

admin := srv.Group("/admin")
admin.Use(middleware.RateLimiter(&middleware.RateLimiterConfig{
    RequestsPerSecond: 100,
    Burst:             200,
}))
```

**配置说明**：
- `RequestsPerSecond`：每秒允许的请求数（令牌补充速率）
- `Burst`：突发流量容量（令牌桶大小），允许短时间内超过平均速率
- `KeyFunc`：生成限流键的函数，默认使用 `c.ClientIP()`
- `OnLimitExceeded`：超出限流时的回调，返回 `nil` 表示已自行处理响应

### 5. 配置管理

```go
// 开发环境
devCfg := &server.Config{
    Addr: ":8080",
    Mode: server.ModeDebug,
    ReadTimeout: 10 * time.Second,
}

// 生产环境
prodCfg := &server.Config{
    Addr:        ":443",
    Mode:        server.ModeRelease,
    ReadTimeout: 30 * time.Second,
    TLS: &server.TLSConfig{
        CertFile: os.Getenv("TLS_CERT"),
        KeyFile:  os.Getenv("TLS_KEY"),
    },
}
```

---

## 迁移指南

### 从原生 Gin 迁移

**Before（Gin）**：
```go
func CreateUser(c *gin.Context) {
    var req CreateUserReq
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    user, err := service.CreateUser(&req)
    if err != nil {
        c.JSON(500, gin.H{"error": "internal error"})
        return
    }
    
    c.JSON(200, gin.H{
        "code": 200,
        "data": user,
    })
}
```

**After（Fox Server）**：
```go
func CreateUser(c *server.Context, req *CreateUserReq) (*User, error) {
    return service.CreateUser(req)
}

// 注册路由
srv.POST("/users", server.BindJSON(CreateUser))
```

**代码减少**：15 行 → 3 行（**80% ↓**）

---

## 常见问题

### Q1: 如何处理组合绑定（JSON + Query）？

**A**: 使用 `Bind` 包装器或手动绑定：

```go
// 方案 1：手动绑定
srv.POST("/users", func(c *server.Context) {
    var req CreateUserReq
    c.ShouldBindJSON(&req)      // JSON
    c.ShouldBindQuery(&req)     // Query
    
    user, err := service.CreateUser(&req)
    if err != nil {
        c.Fail(err)
        return
    }
    c.Ok(user)
})

// 方案 2：拆分结构体
type CreateUserReq struct {
    Body  CreateUserBody  `json:"-"`
    Query CreateUserQuery `form:"-"`
}
```

### Q2: 如何自定义响应格式？

**A**: 使用 `c.JSON()` 直接返回自定义格式：

```go
srv.GET("/custom", func(c *server.Context) {
    c.JSON(200, map[string]any{
        "status": "ok",
        "timestamp": time.Now().Unix(),
    })
})
```

### Q3: 如何访问请求上下文（Context）？

**A**: 第一个参数 `c *server.Context` 提供完整访问：

```go
func GetUser(c *server.Context, req *GetUserReq) (*User, error) {
    // 获取 Header
    token := c.GetHeader("Authorization")
    
    // 获取 IP
    ip := c.ClientIP()
    
    // 设置 Cookie
    c.SetCookie("session", "xxx", 3600, "/", "", false, true)
    
    return user, nil
}
```

### Q4: 如何处理文件上传？

**A**: 使用 `server.Context` 提供的文件上传方法：

```go
srv.POST("/upload", func(c *server.Context) {
    file, _ := c.FormFile("file")
    c.SaveUploadedFile(file, "/tmp/"+file.Filename)
    c.Ok(map[string]string{"filename": file.Filename})
})
```

### Q5: 性能如何？

**A**: 零性能损耗。泛型在编译期展开，运行时与手写代码完全相同。

基准测试数据：
```
BenchmarkBindJSON-10        500000    2467 ns/op    41 allocs/op
BenchmarkTraditional-10     500000    2473 ns/op    41 allocs/op
```

---

## 项目结构

```
core/server/
├── README.md              # 本文档
├── engine.go              # 服务器引擎
├── config.go              # 配置定义
├── context.go             # 上下文封装
├── router.go              # 路由管理
├── handler.go             # 处理器转换
├── wrapper.go             # 泛型包装器
├── response.go            # 响应封装
├── middleware.go          # 内置 Recovery、Tracing 和 Logger 中间件
├── middleware/            # 中间件包
│   ├── ratelimit.go       # 限流
│   └── tracing.go         # 链路追踪（手动指定 provider）
└── example/               # 示例代码
    ├── main.go            # 基础示例
    └── wrapper_main.go    # 包装器示例
```

---

## 相关文档

- [Gin 官方文档](https://gin-gonic.com/docs/)

---

## 许可证

MIT License

---

## 贡献

欢迎提交 Issue 和 Pull Request！

---

**设计理念**：封装而非重写，渐进式增强，生产就绪。
