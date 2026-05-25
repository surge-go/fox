package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/surge-go/fox/pkg/openapi"
	"github.com/surge-go/fox/pkg/openapi/ui"
)

type user struct {
	ID        int64  `json:"id" openapi:"desc=用户 ID" example:"1"`
	Name      string `json:"name" binding:"min=2,max=32" openapi:"desc=用户昵称" example:"Alice"`
	Email     string `json:"email" binding:"email" openapi:"desc=邮箱地址" example:"alice@example.com"`
	Gender    string `json:"gender" binding:"oneof=male female unknown" openapi:"desc=性别" example:"female"`
	Role      string `json:"role" binding:"oneof=admin member" openapi:"desc=用户角色" example:"admin"`
	Status    string `json:"status" binding:"oneof=active disabled" openapi:"desc=用户状态" example:"active"`
	CreatedAt string `json:"createdAt" binding:"datetime" openapi:"desc=创建时间" example:"2026-05-01T09:30:00Z"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email" openapi:"desc=邮箱地址" example:"alice@example.com"`
	Password string `json:"password" binding:"required,min=8" openapi:"desc=登录密码" example:"password"`
}

type createUserRequest struct {
	Name   string `json:"name" binding:"required,min=2,max=32" openapi:"desc=用户昵称" example:"New User"`
	Email  string `json:"email" binding:"required,email" openapi:"desc=邮箱地址" example:"new@example.com"`
	Gender string `json:"gender" binding:"oneof=male female unknown" default:"unknown" openapi:"desc=性别"`
	Role   string `json:"role" binding:"oneof=admin member" default:"member" openapi:"desc=用户角色"`
}

type updateUserRequest struct {
	Name   string `json:"name" binding:"min=2,max=32" openapi:"desc=用户昵称" example:"Alice Updated"`
	Gender string `json:"gender" binding:"oneof=male female unknown" openapi:"desc=性别" example:"female"`
	Status string `json:"status" binding:"oneof=active disabled" openapi:"desc=用户状态" example:"active"`
}

func main() {
	doc := sampleDocument()
	if errs := openapi.Validate(doc); errs.HasErrors() {
		log.Fatalf("invalid OpenAPI document: %v", errs)
	}
	users := sampleUsers()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/auth/login", loginHandler(users))
	mux.HandleFunc("/users", usersHandler(&users))
	mux.HandleFunc("/users/", userHandler(&users))
	mux.HandleFunc("/upload", uploadHandler)
	mux.Handle("/openapi.json", ui.SpecHandler(doc))
	mux.Handle("/", ui.Handler(ui.Config{
		Title:         "OpenAPI 示例",
		SpecURL:       "/openapi.json",
		StoragePrefix: "openapi-example",
	}))

	log.Println("OpenAPI example listening on http://localhost:8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("listen and serve: %v", err)
	}
}

func sampleUsers() []user {
	return []user{
		{ID: 1, Name: "Alice", Email: "alice@example.com", Gender: "female", Role: "admin", Status: "active", CreatedAt: "2026-05-01T09:30:00Z"},
		{ID: 2, Name: "Bob", Email: "bob@example.com", Gender: "male", Role: "member", Status: "active", CreatedAt: "2026-05-10T11:20:00Z"},
		{ID: 3, Name: "Cindy", Email: "cindy@example.com", Gender: "female", Role: "member", Status: "disabled", CreatedAt: "2026-05-18T15:45:00Z"},
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": "1.0.0",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func loginHandler(users []user) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
			return
		}
		if req.Email == "" || req.Password == "" {
			writeError(w, http.StatusBadRequest, "INVALID_CREDENTIALS", "email and password are required")
			return
		}
		current := users[0]
		for _, item := range users {
			if strings.EqualFold(item.Email, req.Email) {
				current = item
				break
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"accessToken": "example-token",
			"expiresIn":   7200,
			"user":        current,
		})
	}
}

func usersHandler(users *[]user) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listUsers(w, r, *users)
		case http.MethodPost:
			createUser(w, r, users)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func listUsers(w http.ResponseWriter, r *http.Request, users []user) {
	status := r.URL.Query().Get("status")
	gender := r.URL.Query().Get("gender")
	if !oneOf(status, "", "all", "active", "disabled") {
		writeError(w, http.StatusBadRequest, "INVALID_STATUS", "status must be all, active, or disabled")
		return
	}
	if !oneOf(gender, "", "all", "male", "female", "unknown") {
		writeError(w, http.StatusBadRequest, "INVALID_GENDER", "gender must be all, male, female, or unknown")
		return
	}
	page := intQuery(r, "page", 1)
	pageSize := intQuery(r, "pageSize", 20)
	filtered := make([]user, 0, len(users))
	for _, item := range users {
		if (status == "" || status == "all" || item.Status == status) &&
			(gender == "" || gender == "all" || item.Gender == gender) {
			filtered = append(filtered, item)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    filtered,
		"total":    len(filtered),
		"page":     page,
		"pageSize": pageSize,
	})
}

func createUser(w http.ResponseWriter, r *http.Request, users *[]user) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if req.Name == "" || req.Email == "" {
		writeError(w, http.StatusBadRequest, "INVALID_USER", "name and email are required")
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}
	if req.Gender == "" {
		req.Gender = "unknown"
	}
	if !oneOf(req.Gender, "male", "female", "unknown") {
		writeError(w, http.StatusBadRequest, "INVALID_GENDER", "gender must be male, female, or unknown")
		return
	}
	if !oneOf(req.Role, "admin", "member") {
		writeError(w, http.StatusBadRequest, "INVALID_ROLE", "role must be admin or member")
		return
	}
	item := user{
		ID:        int64(len(*users) + 1),
		Name:      req.Name,
		Email:     req.Email,
		Gender:    req.Gender,
		Role:      req.Role,
		Status:    "active",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	*users = append(*users, item)
	writeJSON(w, http.StatusCreated, item)
}

func userHandler(users *[]user) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := userIDFromPath(r.URL.Path)
		if !ok {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		index := findUserIndex(*users, id)
		if index < 0 {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, (*users)[index])
		case http.MethodPatch:
			updateUser(w, r, users, index)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func updateUser(w http.ResponseWriter, r *http.Request, users *[]user, index int) {
	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if req.Name != "" {
		(*users)[index].Name = req.Name
	}
	if req.Gender != "" {
		if !oneOf(req.Gender, "male", "female", "unknown") {
			writeError(w, http.StatusBadRequest, "INVALID_GENDER", "gender must be male, female, or unknown")
			return
		}
		(*users)[index].Gender = req.Gender
	}
	if req.Status != "" {
		if !oneOf(req.Status, "active", "disabled") {
			writeError(w, http.StatusBadRequest, "INVALID_STATUS", "status must be active or disabled")
			return
		}
		(*users)[index].Status = req.Status
	}
	writeJSON(w, http.StatusOK, (*users)[index])
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	size, err := io.Copy(io.Discard, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"filename":    header.Filename,
		"size":        size,
		"contentType": header.Header.Get("Content-Type"),
		"note":        r.FormValue("note"),
	})
}

func intQuery(r *http.Request, name string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func userIDFromPath(path string) (int64, bool) {
	raw := strings.TrimPrefix(path, "/users/")
	if raw == "" || strings.Contains(raw, "/") {
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	return id, err == nil && id > 0
}

func findUserIndex(users []user, id int64) int {
	for i, item := range users {
		if item.ID == id {
			return i
		}
	}
	return -1
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"code":    code,
		"message": message,
		"traceId": "trace-example",
	})
}

func sampleDocument() *openapi.Document {
	info := openapi.Info{
		Title:       "OpenAPI 示例",
		Version:     "1.0.0",
		Description: "演示如何通过 Go 启动 OpenAPI UI，并覆盖登录、查询、创建、更新和文件上传等常见接口。",
	}
	servers := []openapi.Server{
		{URL: "http://localhost:8080", Description: "本地开发"},
	}
	schemaBuilder := newExampleSchemaBuilder(info, servers)
	userSchema := mustSchemaOf[user](schemaBuilder)
	loginRequestSchema := mustSchemaOf[loginRequest](schemaBuilder)
	createUserSchema := mustSchemaOf[createUserRequest](schemaBuilder)
	updateUserSchema := mustSchemaOf[updateUserRequest](schemaBuilder)
	healthSchema := mustAddSchema(schemaBuilder, "HealthResponse", objectSchema([]string{"status", "version", "time"}, map[string]openapi.SchemaRef{
		"status":  stringSchema("服务状态。", "ok"),
		"version": stringSchema("服务版本。", "1.0.0"),
		"time":    openapi.SchemaInline(&openapi.Schema{Type: "string", Format: "date-time", Description: "服务器时间。"}),
	}))
	loginResponseSchema := mustAddSchema(schemaBuilder, "LoginResponse", objectSchema([]string{"accessToken", "expiresIn", "user"}, map[string]openapi.SchemaRef{
		"accessToken": stringSchema("访问令牌。", "example-token"),
		"expiresIn":   openapi.SchemaInline(&openapi.Schema{Type: "integer", Format: "int32", Description: "有效期，单位秒。", Example: 7200}),
		"user":        userSchema,
	}))
	userListSchema := mustAddSchema(schemaBuilder, "UserListResponse", objectSchema([]string{"items", "total", "page", "pageSize"}, map[string]openapi.SchemaRef{
		"items":    openapi.SchemaInline(&openapi.Schema{Type: "array", Description: "用户列表。", Items: &userSchema}),
		"total":    openapi.SchemaInline(&openapi.Schema{Type: "integer", Format: "int32", Description: "总数量。", Example: 3}),
		"page":     openapi.SchemaInline(&openapi.Schema{Type: "integer", Format: "int32", Description: "当前页码。", Example: 1}),
		"pageSize": openapi.SchemaInline(&openapi.Schema{Type: "integer", Format: "int32", Description: "每页数量。", Example: 20}),
	}))
	uploadRequestSchema := mustAddSchema(schemaBuilder, "UploadRequest", objectSchema([]string{"file"}, map[string]openapi.SchemaRef{
		"file": openapi.SchemaInline(&openapi.Schema{Type: "string", Format: "binary", Description: "要上传的文件。"}),
		"note": stringSchema("文件备注。", "profile avatar"),
	}))
	uploadResponseSchema := mustAddSchema(schemaBuilder, "UploadResponse", objectSchema([]string{"filename", "size", "contentType"}, map[string]openapi.SchemaRef{
		"filename":    stringSchema("文件名。", "avatar.png"),
		"size":        openapi.SchemaInline(&openapi.Schema{Type: "integer", Format: "int64", Description: "文件大小。", Example: 1024}),
		"contentType": stringSchema("内容类型。", "image/png"),
		"note":        stringSchema("文件备注。", "profile avatar"),
	}))
	errorSchema := mustAddSchema(schemaBuilder, "ErrorResponse", objectSchema([]string{"code", "message"}, map[string]openapi.SchemaRef{
		"code":    stringSchema("错误码。", "INVALID_JSON"),
		"message": stringSchema("错误信息。", "invalid request body"),
		"traceId": stringSchema("链路追踪 ID。", "trace-example"),
	}))
	components := schemaBuilder.Document().Components

	return &openapi.Document{
		OpenAPI: "3.0.3",
		Info:    info,
		Servers: servers,
		Paths: openapi.Paths{
			"/health": {
				Get: &openapi.Operation{
					Tags:        []string{"系统"},
					Groups:      []string{"示例接口", "系统"},
					Summary:     "健康检查",
					OperationID: "getHealth",
					Responses: openapi.Responses{
						"200": openapi.ResponseInline(&openapi.Response{
							Description: "OK",
							Content: map[string]openapi.MediaType{
								"application/json": {Schema: &healthSchema},
							},
						}),
					},
				},
			},
			"/auth/login": {
				Post: &openapi.Operation{
					Tags:        []string{"认证"},
					Groups:      []string{"业务接口", "认证"},
					Summary:     "用户登录",
					OperationID: "login",
					RequestBody: requestBody(loginRequestSchema),
					Responses: openapi.Responses{
						"200": jsonResponse("登录成功", loginResponseSchema),
						"400": jsonResponse("请求参数错误", errorSchema),
					},
				},
			},
			"/users": {
				Get: &openapi.Operation{
					Tags:        []string{"用户"},
					Groups:      []string{"业务接口", "用户"},
					Summary:     "查询用户列表",
					OperationID: "listUsers",
					Parameters: []openapi.ParameterRef{
						openapi.ParameterInline(&openapi.Parameter{Name: "page", In: "query", Description: "页码，从 1 开始。", Schema: schemaPtr(openapi.SchemaInline(&openapi.Schema{Type: "integer", Minimum: floatPtr(1), Default: 1}))}),
						openapi.ParameterInline(&openapi.Parameter{Name: "pageSize", In: "query", Description: "每页数量。", Schema: schemaPtr(openapi.SchemaInline(&openapi.Schema{Type: "integer", Minimum: floatPtr(1), Maximum: floatPtr(100), Default: 20}))}),
						openapi.ParameterInline(&openapi.Parameter{Name: "status", In: "query", Description: "用户状态。", Schema: schemaPtr(openapi.SchemaInline(&openapi.Schema{Type: "string", Enum: []any{"all", "active", "disabled"}, Default: "all"}))}),
						openapi.ParameterInline(&openapi.Parameter{Name: "gender", In: "query", Description: "性别。", Schema: schemaPtr(openapi.SchemaInline(&openapi.Schema{Type: "string", Enum: []any{"all", "male", "female", "unknown"}, Default: "all"}))}),
					},
					Responses: openapi.Responses{
						"200": jsonResponse("查询成功", userListSchema),
					},
				},
				Post: &openapi.Operation{
					Tags:        []string{"用户"},
					Groups:      []string{"业务接口", "用户"},
					Summary:     "创建用户",
					OperationID: "createUser",
					RequestBody: requestBody(createUserSchema),
					Responses: openapi.Responses{
						"201": jsonResponse("创建成功", userSchema),
						"400": jsonResponse("请求参数错误", errorSchema),
					},
				},
			},
			"/users/{id}": {
				Get: &openapi.Operation{
					Tags:        []string{"用户"},
					Groups:      []string{"业务接口", "用户"},
					Summary:     "获取用户详情",
					OperationID: "getUser",
					Parameters:  []openapi.ParameterRef{pathIDParameter()},
					Responses: openapi.Responses{
						"200": jsonResponse("查询成功", userSchema),
						"404": jsonResponse("用户不存在", errorSchema),
					},
				},
				Patch: &openapi.Operation{
					Tags:        []string{"用户"},
					Groups:      []string{"业务接口", "用户"},
					Summary:     "更新用户",
					OperationID: "updateUser",
					Parameters:  []openapi.ParameterRef{pathIDParameter()},
					RequestBody: requestBody(updateUserSchema),
					Responses: openapi.Responses{
						"200": jsonResponse("更新成功", userSchema),
						"400": jsonResponse("请求参数错误", errorSchema),
						"404": jsonResponse("用户不存在", errorSchema),
					},
				},
			},
			"/upload": {
				Post: &openapi.Operation{
					Tags:        []string{"文件"},
					Groups:      []string{"工具接口", "文件"},
					Summary:     "上传文件",
					OperationID: "uploadFile",
					RequestBody: &openapi.RequestBodyRef{Inline: &openapi.RequestBody{
						Required: true,
						Content: map[string]openapi.MediaType{
							"multipart/form-data": {Schema: &uploadRequestSchema},
						},
					}},
					Responses: openapi.Responses{
						"200": jsonResponse("上传成功", uploadResponseSchema),
					},
				},
			},
		},
		Components: components,
	}
}

func newExampleSchemaBuilder(info openapi.Info, servers []openapi.Server) *openapi.Builder {
	return openapi.New(openapi.Config{
		Info:    info,
		Servers: servers,
		TagResolvers: []openapi.TagResolver{
			openapi.DefaultTagResolver(),
			openapi.RuleTagResolver(openapi.RuleTagResolverConfig{TagName: "binding", Strict: true}),
		},
	})
}

func mustSchemaOf[T any](builder *openapi.Builder) openapi.SchemaRef {
	schema, err := openapi.SchemaOf[T](builder)
	if err != nil {
		panic(err)
	}
	return schema
}

func mustAddSchema(builder *openapi.Builder, name string, schema openapi.SchemaRef) openapi.SchemaRef {
	if schema.Inline == nil {
		panic("example schema must be inline: " + name)
	}
	ref, err := builder.AddSchema(name, schema.Inline)
	if err != nil {
		panic(err)
	}
	return ref
}

func requestBody(schema openapi.SchemaRef) *openapi.RequestBodyRef {
	return &openapi.RequestBodyRef{Inline: &openapi.RequestBody{
		Required: true,
		Content: map[string]openapi.MediaType{
			"application/json": {Schema: &schema},
		},
	}}
}

func jsonResponse(description string, schema openapi.SchemaRef) openapi.ResponseRef {
	return openapi.ResponseInline(&openapi.Response{
		Description: description,
		Content: map[string]openapi.MediaType{
			"application/json": {Schema: &schema},
		},
	})
}

func pathIDParameter() openapi.ParameterRef {
	return openapi.ParameterInline(&openapi.Parameter{
		Name:        "id",
		In:          "path",
		Description: "用户 ID。",
		Required:    true,
		Schema:      schemaPtr(openapi.SchemaInline(&openapi.Schema{Type: "integer", Format: "int64", Minimum: floatPtr(1), Example: 1})),
	})
}

func objectSchema(required []string, properties map[string]openapi.SchemaRef) openapi.SchemaRef {
	return openapi.SchemaInline(&openapi.Schema{
		Type:       "object",
		Required:   required,
		Properties: properties,
	})
}

func stringSchema(description, example string) openapi.SchemaRef {
	return openapi.SchemaInline(&openapi.Schema{Type: "string", Description: description, Example: example})
}

func schemaPtr(schema openapi.SchemaRef) *openapi.SchemaRef {
	return &schema
}

func floatPtr(value float64) *float64 {
	return &value
}
