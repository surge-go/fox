package server

// BizHandler 业务处理函数签名，支持自动绑定请求参数和响应包装。
//
// 参数：
//   - c: 完整的 server.Context，可访问所有 HTTP 特性（Header、Cookie、ClientIP 等）
//   - req: 自动绑定的请求参数结构体指针
//
// 返回：
//   - resp: 响应数据结构体指针，会自动包装为标准 Response 格式
//   - error: 业务错误，会自动调用 c.Fail() 处理
//
// 示例：
//
//	func createUser(c *Context, req *CreateUserRequest) (*UserResponse, error) {
//	    clientIP := c.ClientIP()
//	    user, err := userService.Create(c.StdContext(), req.Name, req.Email, clientIP)
//	    if err != nil {
//	        return nil, err
//	    }
//	    return &UserResponse{ID: user.ID, Name: user.Name}, nil
//	}
type BizHandler[Req, Resp any] func(c *Context, req *Req) (*Resp, error)

// BindJSON 自动绑定 JSON 请求体并包装响应。
//
// 工作流程：
//  1. 自动调用 c.BindJSON() 绑定请求体到 Req 结构体
//  2. 绑定失败时自动返回 400 错误（由 BindJSON 内部处理）
//  3. 调用业务处理函数 handler
//  4. 业务函数返回错误时自动调用 c.Fail(err)
//  5. 业务函数成功时自动调用 c.Ok(resp)
//
// 示例：
//
//	type CreateUserRequest struct {
//	    Name  string `json:"name" binding:"required"`
//	    Email string `json:"email" binding:"required,email"`
//	}
//
//	type UserResponse struct {
//	    ID   int64  `json:"id"`
//	    Name string `json:"name"`
//	}
//
//	func createUser(c *Context, req *CreateUserRequest) (*UserResponse, error) {
//	    user, err := userService.Create(c.StdContext(), req.Name, req.Email)
//	    if err != nil {
//	        return nil, err
//	    }
//	    return &UserResponse{ID: user.ID, Name: user.Name}, nil
//	}
//
//	// 路由注册
//	engine.POST("/users", server.BindJSON(createUser))
func BindJSON[Req, Resp any](handler BizHandler[Req, Resp]) HandlerFunc {
	return func(c *Context) {
		var req Req
		if err := c.BindJSON(&req); err != nil {
			return // c.BindJSON 已自动返回 400 错误并中止请求
		}

		resp, err := handler(c, &req)
		if err != nil {
			c.Fail(err)
			return
		}

		c.Ok(resp)
	}
}

// BindQuery 自动绑定 URL 查询参数并包装响应。
//
// 工作流程：
//  1. 自动调用 c.BindQuery() 绑定查询参数到 Req 结构体
//  2. 绑定失败时自动返回 400 错误
//  3. 调用业务处理函数 handler
//  4. 自动处理响应（成功/失败）
//
// 示例：
//
//	type ListUsersQuery struct {
//	    Page     int    `form:"page" binding:"min=1"`
//	    PageSize int    `form:"page_size" binding:"min=1,max=100"`
//	    Keyword  string `form:"keyword"`
//	}
//
//	type ListUsersResponse struct {
//	    Total int64          `json:"total"`
//	    Items []UserResponse `json:"items"`
//	}
//
//	func listUsers(c *Context, req *ListUsersQuery) (*ListUsersResponse, error) {
//	    users, total, err := userService.List(c.StdContext(), req.Page, req.PageSize)
//	    if err != nil {
//	        return nil, err
//	    }
//	    return &ListUsersResponse{Total: total, Items: users}, nil
//	}
//
//	// 路由注册
//	engine.GET("/users", server.BindQuery(listUsers))
func BindQuery[Req, Resp any](handler BizHandler[Req, Resp]) HandlerFunc {
	return func(c *Context) {
		var req Req
		if err := c.BindQuery(&req); err != nil {
			return // c.BindQuery 已自动返回 400 错误并中止请求
		}

		resp, err := handler(c, &req)
		if err != nil {
			c.Fail(err)
			return
		}

		c.Ok(resp)
	}
}

// BindURI 自动绑定 URI 路径参数并包装响应。
//
// 工作流程：
//  1. 自动调用 c.BindURI() 绑定路径参数到 Req 结构体
//  2. 绑定失败时自动返回 400 错误
//  3. 调用业务处理函数 handler
//  4. 自动处理响应（成功/失败）
//
// 示例：
//
//	type GetUserRequest struct {
//	    ID int64 `uri:"id" binding:"required"`
//	}
//
//	type UserResponse struct {
//	    ID   int64  `json:"id"`
//	    Name string `json:"name"`
//	}
//
//	func getUser(c *Context, req *GetUserRequest) (*UserResponse, error) {
//	    user, err := userService.GetByID(c.StdContext(), req.ID)
//	    if err != nil {
//	        return nil, err
//	    }
//	    return &UserResponse{ID: user.ID, Name: user.Name}, nil
//	}
//
//	// 路由注册
//	engine.GET("/users/:id", server.BindURI(getUser))
func BindURI[Req, Resp any](handler BizHandler[Req, Resp]) HandlerFunc {
	return func(c *Context) {
		var req Req
		if err := c.BindURI(&req); err != nil {
			return // c.BindURI 已自动返回 400 错误并中止请求
		}

		resp, err := handler(c, &req)
		if err != nil {
			c.Fail(err)
			return
		}

		c.Ok(resp)
	}
}

// Bind 根据 Content-Type 自动选择绑定方式并包装响应。
//
// 支持的绑定方式：
//   - application/json -> BindJSON
//   - application/x-www-form-urlencoded -> BindForm
//   - multipart/form-data -> BindForm
//   - 其他 -> BindQuery
//
// 注意：不支持组合绑定（如 URI + JSON）。如需组合绑定，请手动调用多次 Bind 方法。
//
// 示例：
//
//	func updateUser(c *Context, req *UpdateUserRequest) (*UserResponse, error) {
//	    user, err := userService.Update(c.StdContext(), req.ID, req.Name)
//	    if err != nil {
//	        return nil, err
//	    }
//	    return &UserResponse{ID: user.ID, Name: user.Name}, nil
//	}
//
//	// 路由注册（自动根据 Content-Type 选择绑定方式）
//	engine.PUT("/users/:id", server.Bind(updateUser))
func Bind[Req, Resp any](handler BizHandler[Req, Resp]) HandlerFunc {
	return func(c *Context) {
		var req Req
		if err := c.Bind(&req); err != nil {
			return // c.Bind 已自动返回 400 错误并中止请求
		}

		resp, err := handler(c, &req)
		if err != nil {
			c.Fail(err)
			return
		}

		c.Ok(resp)
	}
}

// NoReq 无请求参数的 Handler，仅包装响应。
//
// 适用场景：
//   - GET 请求无查询参数（如健康检查）
//   - 不需要绑定请求参数的场景
//
// 示例：
//
//	type HealthResponse struct {
//	    Status  string `json:"status"`
//	    Version string `json:"version"`
//	}
//
//	func healthCheck(c *Context) (*HealthResponse, error) {
//	    c.SetHeader("X-Service-Version", "1.0.0")
//	    return &HealthResponse{Status: "ok", Version: "1.0.0"}, nil
//	}
//
//	// 路由注册
//	engine.GET("/health", server.NoReq(healthCheck))
func NoReq[Resp any](handler func(c *Context) (*Resp, error)) HandlerFunc {
	return func(c *Context) {
		resp, err := handler(c)
		if err != nil {
			c.Fail(err)
			return
		}
		c.Ok(resp)
	}
}

// NoResp 无响应数据的 Handler，自动绑定请求参数。
//
// 适用场景：
//   - DELETE 操作（仅返回成功/失败）
//   - 不需要返回数据的操作
//
// 示例：
//
//	type DeleteUserRequest struct {
//	    ID int64 `uri:"id" binding:"required"`
//	}
//
//	func deleteUser(c *Context, req *DeleteUserRequest) error {
//	    return userService.Delete(c.StdContext(), req.ID)
//	}
//
//	// 路由注册
//	engine.DELETE("/users/:id", server.NoResp(deleteUser))
func NoResp[Req any](handler func(c *Context, req *Req) error) HandlerFunc {
	return func(c *Context) {
		var req Req
		if err := c.Bind(&req); err != nil {
			return // c.Bind 已自动返回 400 错误并中止请求
		}

		if err := handler(c, &req); err != nil {
			c.Fail(err)
			return
		}

		c.Ok(nil)
	}
}

// NoRespJSON 无响应数据的 Handler，自动绑定 JSON 请求体。
//
// 适用场景：
//   - POST/PUT/PATCH 操作不需要返回数据
//   - 仅需要绑定 JSON 请求体
//
// 示例：
//
//	type SendEmailRequest struct {
//	    To      string `json:"to" binding:"required,email"`
//	    Subject string `json:"subject" binding:"required"`
//	    Body    string `json:"body" binding:"required"`
//	}
//
//	func sendEmail(c *Context, req *SendEmailRequest) error {
//	    return emailService.Send(c.StdContext(), req.To, req.Subject, req.Body)
//	}
//
//	// 路由注册
//	engine.POST("/emails/send", server.NoRespJSON(sendEmail))
func NoRespJSON[Req any](handler func(c *Context, req *Req) error) HandlerFunc {
	return func(c *Context) {
		var req Req
		if err := c.BindJSON(&req); err != nil {
			return // c.BindJSON 已自动返回 400 错误并中止请求
		}

		if err := handler(c, &req); err != nil {
			c.Fail(err)
			return
		}

		c.Ok(nil)
	}
}

// NoRespQuery 无响应数据的 Handler，自动绑定 URL 查询参数。
//
// 适用场景：
//   - GET 请求触发操作但不返回数据
//   - 仅需要绑定查询参数
//
// 示例：
//
//	type LogoutRequest struct {
//	    Token string `form:"token" binding:"required"`
//	}
//
//	func logout(c *Context, req *LogoutRequest) error {
//	    return authService.Logout(c.StdContext(), req.Token)
//	}
//
//	// 路由注册
//	engine.GET("/logout", server.NoRespQuery(logout))
func NoRespQuery[Req any](handler func(c *Context, req *Req) error) HandlerFunc {
	return func(c *Context) {
		var req Req
		if err := c.BindQuery(&req); err != nil {
			return // c.BindQuery 已自动返回 400 错误并中止请求
		}

		if err := handler(c, &req); err != nil {
			c.Fail(err)
			return
		}

		c.Ok(nil)
	}
}

// NoRespURI 无响应数据的 Handler，自动绑定 URI 路径参数。
//
// 适用场景：
//   - DELETE 操作（从 URI 获取 ID）
//   - 仅需要绑定路径参数
//
// 示例：
//
//	type DeleteUserRequest struct {
//	    ID int64 `uri:"id" binding:"required"`
//	}
//
//	func deleteUser(c *Context, req *DeleteUserRequest) error {
//	    return userService.Delete(c.StdContext(), req.ID)
//	}
//
//	// 路由注册
//	engine.DELETE("/users/:id", server.NoRespURI(deleteUser))
func NoRespURI[Req any](handler func(c *Context, req *Req) error) HandlerFunc {
	return func(c *Context) {
		var req Req
		if err := c.BindURI(&req); err != nil {
			return // c.BindURI 已自动返回 400 错误并中止请求
		}

		if err := handler(c, &req); err != nil {
			c.Fail(err)
			return
		}

		c.Ok(nil)
	}
}

// Simple 最简单的 Handler，无请求参数，无响应数据。
//
// 适用场景：
//   - 健康检查（仅返回成功）
//   - 触发操作但不需要参数和返回值
//
// 示例：
//
//	func ping(c *Context) error {
//	    c.SetHeader("X-Service-Name", "user-service")
//	    return nil
//	}
//
//	// 路由注册
//	engine.GET("/ping", server.Simple(ping))
func Simple(handler func(c *Context) error) HandlerFunc {
	return func(c *Context) {
		if err := handler(c); err != nil {
			c.Fail(err)
			return
		}
		c.Ok(nil)
	}
}
