package main

import (
	"log"
	"time"

	"github.com/surge-go/fox/core/errors"
	"github.com/surge-go/fox/core/server"
	"github.com/surge-go/fox/core/server/middleware"
)

// ===== 数据模型 =====

type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Age       int       `json:"age"`
	CreatedAt time.Time `json:"created_at"`
}

// ===== 请求/响应结构 =====

type CreateUserRequest struct {
	Name  string `json:"name" binding:"required,min=2,max=50"`
	Email string `json:"email" binding:"required,email"`
	Age   int    `json:"age" binding:"omitempty,min=1,max=150"`
}

type UpdateUserRequest struct {
	Name  string `json:"name" binding:"required,min=2,max=50"`
	Email string `json:"email" binding:"required,email"`
}

type ListUsersQuery struct {
	Page     int    `form:"page" binding:"required,min=1"`
	PageSize int    `form:"page_size" binding:"required,min=1,max=100"`
	Keyword  string `form:"keyword"`
}

type ListUsersResponse struct {
	Total int64  `json:"total"`
	Items []User `json:"items"`
}

type GetUserRequest struct {
	ID int64 `uri:"id" binding:"required,min=1"`
}

type DeleteUserRequest struct {
	ID int64 `uri:"id" binding:"required,min=1"`
}

type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// ===== 模拟数据存储 =====

var (
	users        = make(map[int64]User)
	nextID int64 = 1
)

func init() {
	// 初始化测试数据
	users[1] = User{ID: 1, Name: "Alice", Email: "alice@example.com", Age: 25, CreatedAt: time.Now()}
	users[2] = User{ID: 2, Name: "Bob", Email: "bob@example.com", Age: 30, CreatedAt: time.Now()}
	nextID = 3
}

// ===== 业务处理函数 =====

// createUser 创建用户 - 演示 BindJSON
func createUser(c *server.Context, req *CreateUserRequest) (*User, error) {
	// 访问请求上下文信息
	clientIP := c.ClientIP()
	log.Printf("[createUser] client_ip=%s, name=%s", clientIP, req.Name)

	// 业务校验：检查邮箱是否已存在
	for _, u := range users {
		if u.Email == req.Email {
			return nil, errors.NewWithStatus(1001, 400, "email already exists")
		}
	}

	// 创建用户
	user := User{
		ID:        nextID,
		Name:      req.Name,
		Email:     req.Email,
		Age:       req.Age,
		CreatedAt: time.Now(),
	}
	users[nextID] = user
	nextID++

	// 设置自定义响应头
	c.SetHeader("X-Resource-ID", string(rune(user.ID)))

	return &user, nil
}

// listUsers 用户列表 - 演示 BindQuery
func listUsers(c *server.Context, req *ListUsersQuery) (*ListUsersResponse, error) {
	log.Printf("[listUsers] page=%d, page_size=%d, keyword=%s", req.Page, req.PageSize, req.Keyword)

	// 过滤用户（简单演示）
	var items []User
	for _, u := range users {
		if req.Keyword == "" || contains(u.Name, req.Keyword) || contains(u.Email, req.Keyword) {
			items = append(items, u)
		}
	}

	// 分页（简单演示）
	start := (req.Page - 1) * req.PageSize
	end := start + req.PageSize
	if start >= len(items) {
		items = []User{}
	} else if end > len(items) {
		items = items[start:]
	} else {
		items = items[start:end]
	}

	return &ListUsersResponse{
		Total: int64(len(users)),
		Items: items,
	}, nil
}

// getUser 获取用户详情 - 演示 BindURI
func getUser(c *server.Context, req *GetUserRequest) (*User, error) {
	log.Printf("[getUser] id=%d", req.ID)

	user, exists := users[req.ID]
	if !exists {
		return nil, errors.NewWithStatus(4001, 404, "user not found")
	}

	return &user, nil
}

// updateUser 更新用户 - 演示 URI + JSON 组合绑定
func updateUser(_ *server.Context, uriReq *GetUserRequest, bodyReq *UpdateUserRequest) (*User, error) {
	log.Printf("[updateUser] id=%d, name=%s", uriReq.ID, bodyReq.Name)

	user, exists := users[uriReq.ID]
	if !exists {
		return nil, errors.NewWithStatus(4001, 404, "user not found")
	}

	// 检查邮箱是否被其他用户占用
	for id, u := range users {
		if id != uriReq.ID && u.Email == bodyReq.Email {
			return nil, errors.NewWithStatus(1002, 400, "email already used by another user")
		}
	}

	// 更新用户
	user.Name = bodyReq.Name
	user.Email = bodyReq.Email
	users[uriReq.ID] = user

	return &user, nil
}

// deleteUser 删除用户 - 演示 NoRespURI
func deleteUser(c *server.Context, req *DeleteUserRequest) error {
	log.Printf("[deleteUser] id=%d", req.ID)

	if _, exists := users[req.ID]; !exists {
		return errors.NewWithStatus(4001, 404, "user not found")
	}

	delete(users, req.ID)
	return nil
}

// healthCheck 健康检查 - 演示 NoReq
func healthCheck(c *server.Context) (*HealthResponse, error) {
	c.SetHeader("X-Service-Name", "user-service")
	return &HealthResponse{
		Status:  "ok",
		Version: "1.0.0",
	}, nil
}

// ping 简单 ping - 演示 Simple
func ping(_ *server.Context) error {
	return nil
}

// ===== 中间件示例 =====

// authMiddleware 认证中间件示例
func authMiddleware() server.HandlerFunc {
	return func(c *server.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			c.Fail(errors.NewWithStatus(4010, 401, "missing authorization token"))
			return
		}

		// 简单演示：只检查 token 是否为 "valid-token"
		if token != "Bearer valid-token" {
			c.Fail(errors.NewWithStatus(4011, 401, "invalid token"))
			return
		}

		// 将用户信息存入上下文
		c.Set("user_id", int64(1))
		c.Next()
	}
}

// ===== 主函数 =====

func main() {
	// 1. 创建服务器配置
	cfg := &server.Config{
		Addr:         ":8080",
		Mode:         server.ModeDebug,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// 2. 创建服务器实例
	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// 3. 注册全局中间件（Recovery 已自动注册，Debug/Test 模式默认启用日志）

	// 全局限流：每秒 100 个请求，突发容量 200
	srv.Use(middleware.RateLimiter(&middleware.RateLimiterConfig{
		RequestsPerSecond: 100,
		Burst:             200,
	}))

	// 4. 注册健康检查路由（无需认证）
	srv.GET("/health", server.NoReq(healthCheck))
	srv.GET("/ping", server.Simple(ping))

	// 5. 注册 API 路由组
	api := srv.Group("/api/v1")

	// API 路由组限流：每秒 50 个请求
	api.Use(middleware.RateLimiter(&middleware.RateLimiterConfig{
		RequestsPerSecond: 50,
		Burst:             100,
	}))

	// 用户管理路由（演示 10 种泛型包装器）
	users := api.Group("/users")
	{
		// BindJSON: POST /api/v1/users - 创建用户
		users.POST("", server.BindJSON(createUser))

		// BindQuery: GET /api/v1/users?page=1&page_size=10 - 用户列表
		users.GET("", server.BindQuery(listUsers))

		// BindURI: GET /api/v1/users/:id - 获取用户详情
		users.GET("/:id", server.BindURI(getUser))

		// 组合绑定: PUT /api/v1/users/:id - 更新用户
		users.PUT("/:id", func(c *server.Context) {
			var uriReq GetUserRequest
			if err := c.BindURI(&uriReq); err != nil {
				return
			}

			var bodyReq UpdateUserRequest
			if err := c.BindJSON(&bodyReq); err != nil {
				return
			}

			user, err := updateUser(c, &uriReq, &bodyReq)
			if err != nil {
				c.Fail(err)
				return
			}

			c.Ok(user)
		})

		// NoRespURI: DELETE /api/v1/users/:id - 删除用户
		users.DELETE("/:id", server.NoRespURI(deleteUser))
	}

	// 需要认证的路由组示例
	protected := api.Group("/protected")
	protected.Use(authMiddleware())
	{
		protected.GET("/profile", func(c *server.Context) {
			userID := c.GetInt64("user_id")
			c.Ok(map[string]any{
				"user_id": userID,
				"message": "This is a protected route",
			})
		})
	}

	// 6. 静态文件服务示例（需要创建 public 目录）
	// srv.Static("/static", "./public")
	// srv.StaticFile("/favicon.ico", "./public/favicon.ico")

	// 7. 启动服务器（自动优雅关闭）
	log.Println("========================================")
	log.Println("Fox Server Example")
	log.Println("========================================")
	log.Println("Server starting on http://localhost:8080")
	log.Println("")
	log.Println("Available endpoints:")
	log.Println("  GET    /health                  - Health check")
	log.Println("  GET    /ping                    - Simple ping")
	log.Println("  POST   /api/v1/users            - Create user")
	log.Println("  GET    /api/v1/users            - List users (with pagination)")
	log.Println("  GET    /api/v1/users/:id        - Get user by ID")
	log.Println("  PUT    /api/v1/users/:id        - Update user")
	log.Println("  DELETE /api/v1/users/:id        - Delete user")
	log.Println("  GET    /api/v1/protected/profile - Protected route (requires auth)")
	log.Println("")
	log.Println("Example requests:")
	log.Println("  curl http://localhost:8080/health")
	log.Println("  curl http://localhost:8080/api/v1/users?page=1&page_size=10")
	log.Println("  curl -X POST http://localhost:8080/api/v1/users -H 'Content-Type: application/json' -d '{\"name\":\"Charlie\",\"email\":\"charlie@example.com\",\"age\":28}'")
	log.Println("  curl http://localhost:8080/api/v1/users/1")
	log.Println("  curl -X PUT http://localhost:8080/api/v1/users/1 -H 'Content-Type: application/json' -d '{\"name\":\"Alice Updated\",\"email\":\"alice.new@example.com\"}'")
	log.Println("  curl -X DELETE http://localhost:8080/api/v1/users/1")
	log.Println("  curl http://localhost:8080/api/v1/protected/profile -H 'Authorization: Bearer valid-token'")
	log.Println("")
	log.Println("Press Ctrl+C to gracefully shutdown")
	log.Println("========================================")

	if err := srv.Run(); err != nil {
		log.Printf("Server error: %v", err)
	}

	log.Println("Server exited gracefully")
}

// ===== 辅助函数 =====

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && s[0:len(substr)] == substr) ||
		(len(s) > len(substr) && contains(s[1:], substr)))
}
