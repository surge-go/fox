# OpenAPI

`pkg/openapi` 提供 OpenAPI 3.0.3 文档构建、Go 结构体 schema 生成、文档校验、JSON 编解码和内置调试 UI。

这个包不绑定 `core/server`，也不扫描运行时路由。它负责生成和展示文档，上层可以按自己的路由注册方式把 operation 写入文档。

## 功能

- 构建 OpenAPI 3.0.3 `Document`
- 注册 `paths`、`operations`、`parameters`、`requestBody`、`responses`
- 从 Go struct 生成 OpenAPI schema
- 支持 `binding` tag 解析常见校验规则
- 支持多级分组 `x-groups`
- 校验文档基础结构、重复 `operationId`、重复参数、非法 `$ref`
- 提供可 embed 的 OpenAPI UI
- UI 支持请求调试、全局 Header、响应查看、multipart 文件上传

## 快速开始

```go
package main

import "github.com/surge-go/fox/pkg/openapi"

type CreateUserRequest struct {
	Name   string `json:"name" binding:"required,min=2,max=32" openapi:"desc=用户昵称" example:"Alice"`
	Email  string `json:"email" binding:"required,email" openapi:"desc=邮箱地址" example:"alice@example.com"`
	Gender string `json:"gender" binding:"oneof=male female unknown" default:"unknown" openapi:"desc=性别"`
}

func buildDocument() *openapi.Document {
	builder := openapi.New(openapi.Config{
		Info: openapi.Info{
			Title:   "User API",
			Version: "1.0.0",
		},
		Servers: []openapi.Server{
			{URL: "http://localhost:8080", Description: "本地开发"},
		},
		TagResolvers: []openapi.TagResolver{
			openapi.DefaultTagResolver(),
			openapi.RuleTagResolver(openapi.RuleTagResolverConfig{
				TagName: "binding",
				Strict:  true,
			}),
		},
	})

	createUserSchema, err := openapi.SchemaOf[CreateUserRequest](builder)
	if err != nil {
		panic(err)
	}

	builder.Group("业务接口").
		Group("用户").
		NewOperation("POST", "/users").
		OperationID("createUser").
		Summary("创建用户").
		RequestJSON(createUserSchema, openapi.WithRequestBodyRequired(true)).
		ResponseJSON(201, "创建成功", openapi.SchemaInline(&openapi.Schema{Type: "object"})).
		MustRegister()

	if errs := builder.Validate(); errs.HasErrors() {
		panic(errs)
	}
	return builder.Document()
}
```

## 结构体 tag

### `json`

用于决定 schema 字段名：

```go
Name string `json:"name"`
```

`json:"-"` 会忽略字段。

### `openapi`

用于补充 OpenAPI schema 元数据：

| 写法 | 说明 |
| --- | --- |
| `openapi:"-"` | 忽略字段 |
| `openapi:"required"` | 标记必填 |
| `openapi:"nullable"` | 标记可空 |
| `openapi:"deprecated"` | 标记废弃 |
| `openapi:"name=user_name"` | 覆盖字段名 |
| `openapi:"desc=用户昵称"` | 字段描述 |
| `openapi:"description=用户昵称"` | `desc` 的等价写法 |

示例：

```go
Name string `json:"name" openapi:"required,desc=用户昵称"`
```

### `binding`

通过 `RuleTagResolver` 解析常见校验规则：

```go
openapi.RuleTagResolver(openapi.RuleTagResolverConfig{
	TagName: "binding",
	Strict:  true,
})
```

支持规则：

| 规则 | OpenAPI 映射 |
| --- | --- |
| `required` | `required` |
| `desc=...` / `description=...` | `description` |
| `min=...` | `minimum` / `minLength` / `minItems` |
| `max=...` | `maximum` / `maxLength` / `maxItems` |
| `len=...` | min/max 同值 |
| `oneof=a b c` | `enum` |
| `email` | `format: email` |
| `url` / `uri` | `format: uri` |
| `uuid` | `format: uuid` |
| `datetime` | `format: date-time` |

示例：

```go
Status string `json:"status" binding:"oneof=active disabled" openapi:"desc=用户状态"`
Age    int    `json:"age" binding:"min=1,max=120"`
```

### `example` 和 `default`

```go
Role string `json:"role" binding:"oneof=admin member" default:"member" example:"member"`
```

`example` 和 `default` 会按字段类型做基础转换。

## 分组

`Operation.Groups` 会序列化为 `x-groups`，UI 会按多级分组展示接口。

```go
builder.Group("业务接口").
	Group("用户").
	MustOperation("GET", "/users", operation)
```

如果希望在分组上下文里继续链式配置接口，可以使用 `NewOperation`：

```go
builder.Group("业务接口").
	Group("用户").
	NewOperation("POST", "/users").
	OperationID("createUser").
	Summary("创建用户").
	ResponseJSON(201, "创建成功", openapi.SchemaInline(&openapi.Schema{Type: "object"})).
	MustRegister()
```

也可以在 operation 上直接设置分组：

```go
openapi.NewOperation().
	Groups("业务接口", "用户").
	Summary("查询用户列表")
```

## UI

`pkg/openapi/ui` 提供标准库 `http.Handler`：

```go
package main

import (
	"log"
	"net/http"

	"github.com/surge-go/fox/pkg/openapi/ui"
)

func main() {
	doc := buildDocument()

	mux := http.NewServeMux()
	mux.Handle("/openapi.json", ui.SpecHandler(doc))
	mux.Handle("/", ui.Handler(ui.Config{
		Title:         "OpenAPI 示例",
		SpecURL:       "/openapi.json",
		StoragePrefix: "openapi-example",
	}))

	log.Fatal(http.ListenAndServe(":8080", mux))
}
```

访问：

- UI: `http://localhost:8080/`
- 文档: `http://localhost:8080/openapi.json`

如果文档来自本地 JSON 文件，也可以使用 `ui.FileSpecHandler("openapi.json")` 暴露；如果文档已经在内存中，优先使用 `ui.SpecHandler(doc)`。

运行内置示例：

```bash
go run ./pkg/openapi/example
```

## UI 代理配置

UI 的请求调试通过 Go 代理转发，默认只允许同 host 和 loopback 请求。

```go
ui.Handler(ui.Config{
	SpecURL:               "/openapi.json",
	ProxyHosts:            []string{"localhost", "127.0.0.1"},
	MaxProxyRequestBytes:  64 << 20,
	MaxProxyResponseBytes: 32 << 20,
	MaxProxyFileBytes:     32 << 20,
})
```

代理响应会返回 `durationMs`；当请求耗时小于 1ms 时还会返回 `durationUs`，UI 会优先显示微秒级耗时。

如果只展示文档，不允许 UI 发起真实请求：

```go
ui.Handler(ui.Config{
	SpecURL:      "/openapi.json",
	DisableProxy: true,
})
```

## 文档校验

```go
if errs := openapi.Validate(doc); errs.HasErrors() {
	return errs
}
```

当前校验覆盖：

- `openapi` 版本、`info.title`、`info.version`
- paths 必填和 path 格式
- path 参数声明和 required
- operation responses 必填
- 重复 `operationId`
- 重复参数 `(name, in)`
- 本地 `$ref` 是否存在
- schema 数组 `items`
- schema `required` 字段是否存在
- security scheme 和 OAuth flow 基础字段

## 编解码

```go
data, err := openapi.MarshalIndent(doc, "", "  ")

doc, err := openapi.Unmarshal(data)

doc, err := openapi.ReadFile("openapi.json")
```

## 测试

```bash
go test ./pkg/openapi/...
go vet ./pkg/openapi/...
node --check pkg/openapi/ui/app.js
```

## 设计文档

更多设计细节见 [DESIGN.md](DESIGN.md)。
