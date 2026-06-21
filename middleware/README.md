# Fox Middleware

`middleware` 提供基于 root `fox` 包的可选内置中间件。

## CORS

默认允许所有来源：

```go
app.Use(middleware.CORS())
```

自定义配置：

```go
app.Use(middleware.CORSWithConfig(middleware.CORSConfig{
	AllowOrigins:     []string{"https://example.com", "*.example.com"},
	AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
	AllowHeaders:     []string{"Authorization", "Content-Type"},
	AllowCredentials: true,
	MaxAge:           12 * time.Hour,
}))
```

## RequestID

透传或生成 `X-Request-ID`，并写入 `fox.Context` 和响应头：

```go
app.Use(middleware.RequestID())
```

自定义配置：

```go
app.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{
	Header: "X-Request-ID",
	Generator: func(c *fox.Context) string {
		return "req-" + time.Now().Format("20060102150405.000000000")
	},
}))
```

`Logger` 会读取 `Context.RequestID()`；如果 request id 和 trace id 相同，默认日志只打印一次。

如果同时使用 `Tracing` 和 `RequestID`，推荐先注册 `Tracing`，再注册 `RequestID`。这样没有客户端 `X-Request-ID` 时，`RequestID` 会复用当前 trace id，避免同一次请求出现两个不同的关联 ID：

```go
app.Use(middleware.Tracing())
app.Use(middleware.RequestID())
app.Use(middleware.Logger())
```

## Tracing

创建 HTTP server span，并把 trace id 写入 `fox.Context` 和响应头：

```go
app.Use(middleware.Tracing())
```

自定义配置：

```go
app.Use(middleware.TracingWithConfig(middleware.TracingConfig{
	SkipPaths:     []string{"/health", "/metrics"},
	RecordHeaders: []string{"User-Agent", "X-Request-ID"},
}))
```

`RecordHeaders` 只用于记录非敏感请求头，`Authorization`、`Cookie`、`Set-Cookie`、`Proxy-Authorization`、`X-API-Key` 会被自动忽略。

## Timeout

为请求 context 设置 deadline，处理链返回时如果已超时且还没有写响应，则返回 408：

```go
app.Use(middleware.Timeout())
```

自定义配置：

```go
app.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
	Duration: 3 * time.Second,
}))
```

handler 和 service 应监听 `c.StdContext().Done()` 或传递标准 `context.Context`，不要依赖中间件强制终止 goroutine。

## BodyLimit

限制请求体大小，超过限制返回 413：

```go
app.Use(middleware.BodyLimit())
```

自定义配置：

```go
app.Use(middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{
	MaxBytes: 10 << 20,
}))
```

## Gzip

默认只在客户端声明支持 `gzip`，且响应体达到阈值、响应类型适合压缩时启用：

```go
app.Use(middleware.Gzip())
```

自定义配置：

```go
app.Use(middleware.GzipWithConfig(middleware.GzipConfig{
	Level:     gzip.BestSpeed,
	MinSize:   512,
	SkipPaths: []string{"/health", "/metrics"},
}))
```

## RateLimit

默认按客户端 IP 限流，使用本地内存令牌桶：

```go
app.Use(middleware.RateLimit())
```

自定义配置：

```go
app.Use(middleware.RateLimitWithConfig(middleware.RateLimitConfig{
	Limit:  100,
	Window: time.Minute,
	Burst:  20,
	KeyFunc: func(c *fox.Context) string {
		return c.ClientIP()
	},
}))
```

使用 Redis 实例做多进程/多实例共享限流：

```go
client, err := redis.NewClient(&redis.Config{
	Addrs: []string{"localhost:6379"},
})
if err != nil {
	return err
}

app.Use(middleware.RateLimitWithConfig(middleware.RateLimitConfig{
	Redis:  client,
	Limit:  100,
	Window: time.Minute,
	Burst:  20,
}))
```
