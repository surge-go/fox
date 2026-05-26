package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/surge-go/fox/pkg/openapi"
)

type filePathRequest struct {
	Filepath string `uri:"filepath" binding:"required"`
}

func TestOpenAPIDocumentFromOfficialWrappers(t *testing.T) {
	engine, err := New(&Config{
		Addr: ":8080",
		Mode: ModeTest,
		OpenAPI: &OpenAPIConfig{
			Info: openapi.Info{Title: "User API", Version: "1.0.0"},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	api := engine.Group("/api/v1").DocGroup(DocGroup{Groups: []string{"业务接口", "用户"}})
	api.POST("/users", BindJSON(func(c *Context, req *CreateUserRequest) (*UserResponse, error) {
		return &UserResponse{ID: 1, Name: req.Name, Email: req.Email}, nil
	})).
		OperationID("createUser").
		Summary("创建用户").
		Tags("用户").
		Security("bearer")

	api.GET("/users", BindQuery(func(c *Context, req *ListUsersQuery) (*ListUsersResponse, error) {
		return &ListUsersResponse{}, nil
	})).
		Summary("查询用户").
		Tags("用户")

	api.GET("/users/:id", BindURI(func(c *Context, req *GetUserRequest) (*UserResponse, error) {
		return &UserResponse{ID: req.ID}, nil
	})).
		Summary("获取用户详情")

	api.DELETE("/users/:id", NoRespURI(func(c *Context, req *DeleteUserRequest) error {
		return nil
	})).
		Summary("删除用户")

	api.GET("/files/*filepath", BindURI(func(c *Context, req *filePathRequest) (*UserResponse, error) {
		return &UserResponse{Name: req.Filepath}, nil
	})).
		Summary("获取文件")

	doc := engine.OpenAPIDocument()
	if doc == nil {
		t.Fatal("OpenAPIDocument() = nil")
	}
	if errs := openapi.Validate(doc); errs.HasErrors() {
		t.Fatalf("OpenAPIDocument() validation errors = %v", errs)
	}

	users := doc.Paths["/api/v1/users"]
	if users.Post == nil {
		t.Fatal("POST /api/v1/users missing")
	}
	if got, want := users.Post.OperationID, "createUser"; got != want {
		t.Fatalf("operationId = %q, want %q", got, want)
	}
	if got, want := users.Post.Summary, "创建用户"; got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	if len(users.Post.Tags) != 1 || users.Post.Tags[0] != "用户" {
		t.Fatalf("tags = %#v, want 用户", users.Post.Tags)
	}
	if len(users.Post.Groups) != 2 || users.Post.Groups[0] != "业务接口" || users.Post.Groups[1] != "用户" {
		t.Fatalf("groups = %#v, want inherited doc groups", users.Post.Groups)
	}
	if users.Post.RequestBody == nil || users.Post.RequestBody.Inline == nil {
		t.Fatal("POST request body missing")
	}
	if !users.Post.RequestBody.Inline.Required {
		t.Fatal("POST request body required = false, want true")
	}
	if len(users.Post.Security) != 1 {
		t.Fatalf("security = %#v, want one requirement", users.Post.Security)
	}

	if users.Get == nil {
		t.Fatal("GET /api/v1/users missing")
	}
	pageParam := findParameter(users.Get.Parameters, "query", "page")
	if pageParam == nil {
		t.Fatal("query parameter page missing")
	}
	if pageParam.Schema == nil || pageParam.Schema.Inline == nil || pageParam.Schema.Inline.Minimum == nil || *pageParam.Schema.Inline.Minimum != 1 {
		t.Fatalf("page schema = %#v, want minimum 1", pageParam.Schema)
	}
	pageSizeParam := findParameter(users.Get.Parameters, "query", "page_size")
	if pageSizeParam == nil {
		t.Fatal("query parameter page_size missing")
	}
	if pageSizeParam.Schema == nil || pageSizeParam.Schema.Inline == nil || pageSizeParam.Schema.Inline.Maximum == nil || *pageSizeParam.Schema.Inline.Maximum != 100 {
		t.Fatalf("page_size schema = %#v, want maximum 100", pageSizeParam.Schema)
	}

	userByID := doc.Paths["/api/v1/users/{id}"]
	if userByID.Get == nil {
		t.Fatal("GET /api/v1/users/{id} missing")
	}
	idParam := findParameter(userByID.Get.Parameters, "path", "id")
	if idParam == nil {
		t.Fatal("path parameter id missing")
	}
	if !idParam.Required {
		t.Fatal("path parameter id required = false, want true")
	}
	if idParam.Schema == nil || idParam.Schema.Inline == nil || idParam.Schema.Inline.Format != "int64" {
		t.Fatalf("id schema = %#v, want int64", idParam.Schema)
	}

	if userByID.Delete == nil {
		t.Fatal("DELETE /api/v1/users/{id} missing")
	}
	dataSchema := responseDataSchema(t, userByID.Delete.Responses["200"])
	if dataSchema.Inline == nil || !dataSchema.Inline.Nullable {
		t.Fatalf("delete response data schema = %#v, want nullable data", dataSchema)
	}

	files := doc.Paths["/api/v1/files/{filepath}"]
	if files.Get == nil {
		t.Fatal("GET /api/v1/files/{filepath} missing")
	}
	filepathParam := findParameter(files.Get.Parameters, "path", "filepath")
	if filepathParam == nil || !filepathParam.Required {
		t.Fatalf("filepath parameter = %#v, want required path parameter", filepathParam)
	}

	first, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal first doc: %v", err)
	}
	second, err := json.Marshal(engine.OpenAPIDocument())
	if err != nil {
		t.Fatalf("marshal second doc: %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("OpenAPIDocument() is not stable across repeated calls")
	}
}

func TestOpenAPIDocumentSkipsPlainHandlers(t *testing.T) {
	engine, err := New(&Config{
		Addr:    ":8080",
		Mode:    ModeTest,
		OpenAPI: &OpenAPIConfig{Info: openapi.Info{Title: "API", Version: "1.0.0"}},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	engine.GET("/manual", func(c *Context) {
		c.String(http.StatusNoContent, "")
	}).Summary("手写接口")

	doc := engine.OpenAPIDocument()
	if _, ok := doc.Paths["/manual"]; ok {
		t.Fatal("plain handler should not be included in OpenAPI document")
	}
}

func findParameter(params []openapi.ParameterRef, in, name string) *openapi.Parameter {
	for _, ref := range params {
		if ref.Inline != nil && ref.Inline.In == in && ref.Inline.Name == name {
			return ref.Inline
		}
	}
	return nil
}

func responseDataSchema(t *testing.T, ref openapi.ResponseRef) openapi.SchemaRef {
	t.Helper()
	if ref.Inline == nil {
		t.Fatal("response is not inline")
	}
	mt, ok := ref.Inline.Content["application/json"]
	if !ok || mt.Schema == nil || mt.Schema.Inline == nil {
		t.Fatalf("json response schema missing: %#v", ref.Inline.Content)
	}
	data, ok := mt.Schema.Inline.Properties["data"]
	if !ok {
		t.Fatal("response data property missing")
	}
	return data
}
