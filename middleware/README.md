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
