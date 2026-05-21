package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/surge-go/fox/core/errors"
)

// ===== 测试用结构体 =====

type CreateUserRequest struct {
	Name  string `json:"name" binding:"required"`
	Email string `json:"email" binding:"required,email"`
}

type UserResponse struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type ListUsersQuery struct {
	Page     int    `form:"page" binding:"min=1"`
	PageSize int    `form:"page_size" binding:"min=1,max=100"`
	Keyword  string `form:"keyword"`
}

type ListUsersResponse struct {
	Total int64          `json:"total"`
	Items []UserResponse `json:"items"`
}

type GetUserRequest struct {
	ID int64 `uri:"id" binding:"required"`
}

type DeleteUserRequest struct {
	ID int64 `uri:"id" binding:"required"`
}

// ===== BindJSON 测试 =====

func TestBindJSON_Success(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	handler := BindJSON(func(c *Context, req *CreateUserRequest) (*UserResponse, error) {
		if req.Name != "Alice" {
			t.Errorf("expected name 'Alice', got %q", req.Name)
		}
		if req.Email != "alice@example.com" {
			t.Errorf("expected email 'alice@example.com', got %q", req.Email)
		}

		clientIP := c.ClientIP()
		if clientIP == "" {
			t.Error("expected non-empty client IP")
		}

		return &UserResponse{
			ID:    1,
			Name:  req.Name,
			Email: req.Email,
		}, nil
	})

	engine.POST("/users", handler)

	body := `{"name":"Alice","email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), `"id":1`) {
		t.Errorf("expected response to contain user data, got %s", w.Body.String())
	}
}

func TestBindJSON_BindError(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	handler := BindJSON(func(c *Context, req *CreateUserRequest) (*UserResponse, error) {
		t.Error("handler should not be called when bind fails")
		return nil, nil
	})

	engine.POST("/users", handler)

	body := `{"name":""}` // missing required email
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestBindJSON_HandlerError(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	handler := BindJSON(func(c *Context, req *CreateUserRequest) (*UserResponse, error) {
		return nil, errors.NewWithStatus(1001, 400, "user already exists")
	})

	engine.POST("/users", handler)

	body := `{"name":"Alice","email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), "user already exists") {
		t.Errorf("expected error message in response, got %s", w.Body.String())
	}
}

// ===== BindQuery 测试 =====

func TestBindQuery_Success(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	handler := BindQuery(func(c *Context, req *ListUsersQuery) (*ListUsersResponse, error) {
		if req.Page != 1 {
			t.Errorf("expected page 1, got %d", req.Page)
		}
		if req.PageSize != 10 {
			t.Errorf("expected page_size 10, got %d", req.PageSize)
		}
		if req.Keyword != "alice" {
			t.Errorf("expected keyword 'alice', got %q", req.Keyword)
		}

		return &ListUsersResponse{
			Total: 1,
			Items: []UserResponse{
				{ID: 1, Name: "Alice", Email: "alice@example.com"},
			},
		}, nil
	})

	engine.GET("/users", handler)

	req := httptest.NewRequest(http.MethodGet, "/users?page=1&page_size=10&keyword=alice", nil)
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), `"total":1`) {
		t.Errorf("expected response to contain total, got %s", w.Body.String())
	}
}

func TestBindQuery_ValidationError(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	handler := BindQuery(func(c *Context, req *ListUsersQuery) (*ListUsersResponse, error) {
		t.Error("handler should not be called when validation fails")
		return nil, nil
	})

	engine.GET("/users", handler)

	req := httptest.NewRequest(http.MethodGet, "/users?page=0&page_size=10", nil) // page < 1
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// ===== BindURI 测试 =====

func TestBindURI_Success(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	handler := BindURI(func(c *Context, req *GetUserRequest) (*UserResponse, error) {
		if req.ID != 123 {
			t.Errorf("expected ID 123, got %d", req.ID)
		}

		return &UserResponse{
			ID:    req.ID,
			Name:  "Alice",
			Email: "alice@example.com",
		}, nil
	})

	engine.GET("/users/:id", handler)

	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), `"id":123`) {
		t.Errorf("expected response to contain ID 123, got %s", w.Body.String())
	}
}

// ===== Bind 测试 =====

func TestBind_AutoDetect(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	type TestRequest struct {
		Name string `json:"name" form:"name"`
	}

	handler := Bind(func(c *Context, req *TestRequest) (*struct{ Name string `json:"name"` }, error) {
		return &struct{ Name string `json:"name"` }{Name: req.Name}, nil
	})

	engine.POST("/test", handler)

	body := `{"name":"json-test"}`
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), "json-test") {
		t.Errorf("expected response to contain 'json-test', got %s", w.Body.String())
	}
}

// ===== NoReq 测试 =====

func TestNoReq_Success(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	handler := NoReq(func(c *Context) (*struct{ Status string `json:"status"` }, error) {
		c.SetHeader("X-Service-Version", "1.0.0")
		return &struct{ Status string `json:"status"` }{Status: "ok"}, nil
	})

	engine.GET("/health", handler)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Header().Get("X-Service-Version") != "1.0.0" {
		t.Errorf("expected header X-Service-Version=1.0.0, got %s", w.Header().Get("X-Service-Version"))
	}

	if !strings.Contains(w.Body.String(), `"status":"ok"`) {
		t.Errorf("expected response to contain status ok, got %s", w.Body.String())
	}
}

func TestNoReq_HandlerError(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	handler := NoReq(func(c *Context) (*struct{ Status string `json:"status"` }, error) {
		return nil, errors.NewWithStatus(5001, 500, "service unavailable")
	})

	engine.GET("/health", handler)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

// ===== NoResp 测试 =====

func TestNoResp_Success(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	type TestRequest struct {
		Name string `json:"name"`
	}

	handler := NoResp(func(c *Context, req *TestRequest) error {
		if req.Name != "test" {
			t.Errorf("expected name 'test', got %q", req.Name)
		}
		return nil
	})

	engine.POST("/test", handler)

	body := `{"name":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), `"data":null`) {
		t.Errorf("expected data to be null, got %s", w.Body.String())
	}
}

func TestNoRespJSON_Success(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	type TestRequest struct {
		Name string `json:"name"`
	}

	handler := NoRespJSON(func(c *Context, req *TestRequest) error {
		if req.Name != "test" {
			t.Errorf("expected name 'test', got %q", req.Name)
		}
		return nil
	})

	engine.POST("/test", handler)

	body := `{"name":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestNoRespQuery_Success(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	type TestRequest struct {
		Name string `form:"name"`
	}

	handler := NoRespQuery(func(c *Context, req *TestRequest) error {
		if req.Name != "test" {
			t.Errorf("expected name 'test', got %q", req.Name)
		}
		return nil
	})

	engine.GET("/test", handler)

	req := httptest.NewRequest(http.MethodGet, "/test?name=test", nil)
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestNoRespURI_Success(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	handler := NoRespURI(func(c *Context, req *DeleteUserRequest) error {
		if req.ID != 123 {
			t.Errorf("expected ID 123, got %d", req.ID)
		}
		return nil
	})

	engine.DELETE("/users/:id", handler)

	req := httptest.NewRequest(http.MethodDelete, "/users/123", nil)
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), `"data":null`) {
		t.Errorf("expected response data to be null, got %s", w.Body.String())
	}
}

func TestNoRespURI_HandlerError(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	handler := NoRespURI(func(c *Context, req *DeleteUserRequest) error {
		return errors.NewWithStatus(4001, 404, "user not found")
	})

	engine.DELETE("/users/:id", handler)

	req := httptest.NewRequest(http.MethodDelete, "/users/123", nil)
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), "user not found") {
		t.Errorf("expected error message in response, got %s", w.Body.String())
	}
}

// ===== Simple 测试 =====

func TestSimple_Success(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	handler := Simple(func(c *Context) error {
		c.SetHeader("X-Service-Name", "user-service")
		return nil
	})

	engine.GET("/ping", handler)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Header().Get("X-Service-Name") != "user-service" {
		t.Errorf("expected header X-Service-Name=user-service, got %s", w.Header().Get("X-Service-Name"))
	}
}

func TestSimple_HandlerError(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	handler := Simple(func(c *Context) error {
		return errors.NewWithStatus(5003, 503, "service temporarily unavailable")
	})

	engine.GET("/ping", handler)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

// ===== 边缘情况测试 =====

func TestBindJSON_PointerType(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	type PtrRequest struct {
		Name string `json:"name"`
	}

	handler := BindJSON(func(c *Context, req **PtrRequest) (*struct{ OK bool `json:"ok"` }, error) {
		if req == nil || *req == nil {
			t.Error("req should not be nil")
		}
		return &struct{ OK bool `json:"ok"` }{OK: true}, nil
	})

	engine.POST("/test", handler)

	body := `{"name":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestBindJSON_EmptyStruct(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	handler := BindJSON(func(c *Context, req *struct{}) (*struct{ OK bool `json:"ok"` }, error) {
		return &struct{ OK bool `json:"ok"` }{OK: true}, nil
	})

	engine.POST("/test", handler)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestBindJSON_VariousTypes(t *testing.T) {
	cfg := &Config{Addr: ":8080", Mode: ModeTest}
	engine, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	tests := []struct {
		name    string
		handler HandlerFunc
		body    string
		wantErr bool
	}{
		{
			name: "struct_type",
			handler: BindJSON(func(c *Context, req *struct{ Name string `json:"name"` }) (*struct{ OK bool `json:"ok"` }, error) {
				return &struct{ OK bool `json:"ok"` }{OK: true}, nil
			}),
			body:    `{"name":"test"}`,
			wantErr: false,
		},
		{
			name: "map_type",
			handler: BindJSON(func(c *Context, req *map[string]string) (*struct{ OK bool `json:"ok"` }, error) {
				if req == nil {
					t.Error("req should not be nil")
				}
				if *req == nil {
					t.Error("*req should not be nil after binding")
				}
				return &struct{ OK bool `json:"ok"` }{OK: true}, nil
			}),
			body:    `{"key":"value"}`,
			wantErr: false,
		},
		{
			name: "slice_type",
			handler: BindJSON(func(c *Context, req *[]string) (*struct{ OK bool `json:"ok"` }, error) {
				if req == nil {
					t.Error("req should not be nil")
				}
				return &struct{ OK bool `json:"ok"` }{OK: true}, nil
			}),
			body:    `["a","b","c"]`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine.POST("/test/"+tt.name, tt.handler)

			req := httptest.NewRequest(http.MethodPost, "/test/"+tt.name, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			defer func() {
				if r := recover(); r != nil {
					if !tt.wantErr {
						t.Errorf("unexpected panic: %v", r)
					}
				}
			}()

			engine.ServeHTTP(w, req)

			if !tt.wantErr && w.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
			}
		})
	}
}
