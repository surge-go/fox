# OpenAPI Package Design

## 背景

`pkg/openapi` 是 fox 项目里的独立 OpenAPI 3 支持包。它只关注两件事：

1. 构建、校验、序列化 OpenAPI 3 文档。
2. 提供内置文档 UI，用于浏览 OpenAPI 文档。

第一版不和 `core/server` 绑定，不负责路由注册、请求绑定、业务响应包装，也不从任何运行时 HTTP 框架里反推接口信息。业务层或上层适配包可以在后续基于 `pkg/openapi` 构建 server 集成，但那不属于本包第一阶段范围。

## 目标

- 支持 OpenAPI 3.0.3 文档模型。
- 支持手动构建 paths、operations、parameters、request bodies、responses、components、security schemes、tags、servers。
- 支持从 Go 类型生成 JSON Schema 风格的 OpenAPI schema。
- 支持 JSON 序列化输出。
- 支持读取已有 OpenAPI 3 JSON 文档，解析为 `Document`。
- 支持文档基础校验，尽早发现缺失 `info`、非法 path、重复 operationId、无效 `$ref` 等问题。
- 提供 `openapi/ui` 自研前端页面，支持加载并展示 OpenAPI 3 JSON 文档。
- UI 资源可 embed 到 Go 二进制，并提供标准库 `http.Handler`，但不依赖任何具体 Web 框架。

## 非目标

- 不集成 `core/server`。
- 不注册业务接口路由。
- 不提供请求绑定、响应包装、运行时参数校验。
- 不从 gin、net/http 或其他路由树自动扫描接口。
- 不扫描 Go 源码注释生成文档。
- 不做 SDK 生成。
- 第一版不实现 OpenAPI 3.1，后续可以通过版本抽象扩展。

## 目录结构

```text
pkg/openapi/
  DESIGN.md
  document.go       # OpenAPI 3 文档结构体
  builder.go        # Builder、文档构建入口
  operation.go      # Operation、Parameter、RequestBody、Response 构建
  schema.go         # Go 类型到 OpenAPI schema 的转换
  ref.go            # SchemaRef、ResponseRef 等引用类型定义和序列化
  validate.go       # 文档基础校验
  encoding.go       # Marshal、Unmarshal 和自定义序列化辅助
  ui/
    embed.go        # embed 前端静态资源
    handler.go      # 标准库 http.Handler
    index.html
    app.css
    app.js
```

## 核心模型

第一版以 OpenAPI 3.0.3 为基准，结构体字段尽量和规范命名保持一致。Go 字段使用导出名，JSON tag 使用 OpenAPI 字段名。

```go
type Document struct {
    OpenAPI    string               `json:"openapi"`
    Info       Info                 `json:"info"`
    Servers    []Server             `json:"servers,omitempty"`
    Paths      Paths                `json:"paths"`
    Components *Components          `json:"components,omitempty"`
    Security   []SecurityRequirement `json:"security,omitempty"`
    Tags       []Tag                `json:"tags,omitempty"`
    ExternalDocs *ExternalDocs      `json:"externalDocs,omitempty"`
}
```

`Document.OpenAPI` 为空时由 Builder 默认填充为 `3.0.3`。

### 数据模型完整定义

第一版需要覆盖 OpenAPI 3.0.3 的核心对象。暂不实现的对象仍可保留字段类型，便于手动构建文档；Builder 辅助 API 可以先只覆盖常用路径。

```go
type Info struct {
    Title          string   `json:"title"`
    Version        string   `json:"version"`
    Description    string   `json:"description,omitempty"`
    TermsOfService string   `json:"termsOfService,omitempty"`
    Contact        *Contact `json:"contact,omitempty"`
    License        *License `json:"license,omitempty"`
}

type Contact struct {
    Name  string `json:"name,omitempty"`
    URL   string `json:"url,omitempty"`
    Email string `json:"email,omitempty"`
}

type License struct {
    Name string `json:"name"`
    URL  string `json:"url,omitempty"`
}

type Server struct {
    URL         string                    `json:"url"`
    Description string                    `json:"description,omitempty"`
    Variables   map[string]ServerVariable `json:"variables,omitempty"`
}

type ServerVariable struct {
    Enum        []string `json:"enum,omitempty"`
    Default     string   `json:"default"`
    Description string   `json:"description,omitempty"`
}

type Tag struct {
    Name         string        `json:"name"`
    Description  string        `json:"description,omitempty"`
    ExternalDocs *ExternalDocs `json:"externalDocs,omitempty"`
}

type ExternalDocs struct {
    Description string `json:"description,omitempty"`
    URL         string `json:"url"`
}

type SecurityRequirement map[string][]string
```

Paths 和 operation：

```go
type Paths map[string]PathItem

type PathItem struct {
    Ref         string         `json:"$ref,omitempty"`
    Summary     string         `json:"summary,omitempty"`
    Description string         `json:"description,omitempty"`
    Get         *Operation     `json:"get,omitempty"`
    Put         *Operation     `json:"put,omitempty"`
    Post        *Operation     `json:"post,omitempty"`
    Delete      *Operation     `json:"delete,omitempty"`
    Options     *Operation     `json:"options,omitempty"`
    Head        *Operation     `json:"head,omitempty"`
    Patch       *Operation     `json:"patch,omitempty"`
    Trace       *Operation     `json:"trace,omitempty"`
    Servers     []Server       `json:"servers,omitempty"`
    Parameters  []ParameterRef `json:"parameters,omitempty"`
}

type Operation struct {
    Tags        []string              `json:"tags,omitempty"`
    Summary     string                `json:"summary,omitempty"`
    Description string                `json:"description,omitempty"`
    OperationID string                `json:"operationId,omitempty"`
    Parameters  []ParameterRef        `json:"parameters,omitempty"`
    RequestBody *RequestBodyRef       `json:"requestBody,omitempty"`
    Responses   Responses             `json:"responses"`
    Deprecated  bool                  `json:"deprecated,omitempty"`
    Security    []SecurityRequirement `json:"security,omitempty"`
    Servers     []Server              `json:"servers,omitempty"`
}

type Responses map[string]ResponseRef
```

Parameter、request body 和 response：

```go
type Parameter struct {
    Name        string               `json:"name"`
    In          string               `json:"in"` // "query", "header", "path", "cookie"
    Description string               `json:"description,omitempty"`
    Required    bool                 `json:"required,omitempty"`
    Deprecated  bool                 `json:"deprecated,omitempty"`
    Schema      *SchemaRef           `json:"schema,omitempty"`
    Content     map[string]MediaType `json:"content,omitempty"`
    Style       string               `json:"style,omitempty"`
    Explode     *bool                `json:"explode,omitempty"`
}

type RequestBody struct {
    Description string               `json:"description,omitempty"`
    Content     map[string]MediaType `json:"content"`
    Required    bool                 `json:"required,omitempty"`
}

type Response struct {
    Description string               `json:"description"`
    Headers     map[string]HeaderRef `json:"headers,omitempty"`
    Content     map[string]MediaType `json:"content,omitempty"`
    Links       map[string]LinkRef   `json:"links,omitempty"`
}

type MediaType struct {
    Schema   *SchemaRef            `json:"schema,omitempty"`
    Example  any                   `json:"example,omitempty"`
    Examples map[string]ExampleRef `json:"examples,omitempty"`
    Encoding map[string]Encoding   `json:"encoding,omitempty"`
}

type Header struct {
    Description string               `json:"description,omitempty"`
    Required    bool                 `json:"required,omitempty"`
    Deprecated  bool                 `json:"deprecated,omitempty"`
    Schema      *SchemaRef           `json:"schema,omitempty"`
    Content     map[string]MediaType `json:"content,omitempty"`
}
```

Schema 和 components：

```go
type Schema struct {
    Type                 string                `json:"type,omitempty"`
    Format               string                `json:"format,omitempty"`
    Title                string                `json:"title,omitempty"`
    Description          string                `json:"description,omitempty"`
    Properties           map[string]SchemaRef  `json:"properties,omitempty"`
    Required             []string              `json:"required,omitempty"`
    Items                *SchemaRef            `json:"items,omitempty"`
    AdditionalProperties *AdditionalProperties `json:"additionalProperties,omitempty"`
    Nullable             bool                  `json:"nullable,omitempty"`
    ReadOnly             bool                  `json:"readOnly,omitempty"`
    WriteOnly            bool                  `json:"writeOnly,omitempty"`
    Deprecated           bool                  `json:"deprecated,omitempty"`
    Minimum              *float64              `json:"minimum,omitempty"`
    Maximum              *float64              `json:"maximum,omitempty"`
    ExclusiveMinimum     bool                  `json:"exclusiveMinimum,omitempty"`
    ExclusiveMaximum     bool                  `json:"exclusiveMaximum,omitempty"`
    MinLength            *int                  `json:"minLength,omitempty"`
    MaxLength            *int                  `json:"maxLength,omitempty"`
    Pattern              string                `json:"pattern,omitempty"`
    MinItems             *int                  `json:"minItems,omitempty"`
    MaxItems             *int                  `json:"maxItems,omitempty"`
    Enum                 []any                 `json:"enum,omitempty"`
    Default              any                   `json:"default,omitempty"`
    Example              any                   `json:"example,omitempty"`
    AllOf                []SchemaRef           `json:"allOf,omitempty"`
    OneOf                []SchemaRef           `json:"oneOf,omitempty"`
    AnyOf                []SchemaRef           `json:"anyOf,omitempty"`
    Not                  *SchemaRef            `json:"not,omitempty"`
}

type Components struct {
    Schemas         map[string]SchemaRef         `json:"schemas,omitempty"`
    Responses       map[string]ResponseRef       `json:"responses,omitempty"`
    Parameters      map[string]ParameterRef      `json:"parameters,omitempty"`
    RequestBodies   map[string]RequestBodyRef    `json:"requestBodies,omitempty"`
    Headers         map[string]HeaderRef         `json:"headers,omitempty"`
    Examples        map[string]ExampleRef        `json:"examples,omitempty"`
    SecuritySchemes map[string]SecuritySchemeRef `json:"securitySchemes,omitempty"`
    Links           map[string]LinkRef           `json:"links,omitempty"`
    Callbacks       map[string]CallbackRef       `json:"callbacks,omitempty"`
}
```

第一版 Builder 辅助方法优先支持 `schemas`、`responses`、`parameters`、`requestBodies`、`headers`、`securitySchemes`。`examples`、`links`、`callbacks` 结构体先保留，复杂构建器后续补。模型层必须完整支持 Reference Object，保证读取已有 OpenAPI JSON 时不会丢失 `$ref`。

支撑对象：

```go
type Example struct {
    Summary       string `json:"summary,omitempty"`
    Description   string `json:"description,omitempty"`
    Value         any    `json:"value,omitempty"`
    ExternalValue string `json:"externalValue,omitempty"`
}

type Encoding struct {
    ContentType   string               `json:"contentType,omitempty"`
    Headers       map[string]HeaderRef `json:"headers,omitempty"`
    Style         string               `json:"style,omitempty"`
    Explode       *bool                `json:"explode,omitempty"`
    AllowReserved bool                 `json:"allowReserved,omitempty"`
}

type Link struct {
    OperationRef string         `json:"operationRef,omitempty"`
    OperationID  string         `json:"operationId,omitempty"`
    Parameters   map[string]any `json:"parameters,omitempty"`
    RequestBody  any            `json:"requestBody,omitempty"`
    Description  string         `json:"description,omitempty"`
    Server       *Server        `json:"server,omitempty"`
}

type Callback map[string]PathItem

type AdditionalProperties struct {
    Allowed *bool
    Schema  *SchemaRef
}

type SchemaRef struct {
    Ref    string
    Inline *Schema
}

type ResponseRef struct {
    Ref    string
    Inline *Response
}

// ParameterRef、RequestBodyRef、HeaderRef、ExampleRef、LinkRef、
// CallbackRef、SecuritySchemeRef 使用同样结构，只是 Inline 类型不同。

type SecurityScheme struct {
    Type             string      `json:"type"`
    Description      string      `json:"description,omitempty"`
    Name             string      `json:"name,omitempty"`
    In               string      `json:"in,omitempty"`
    Scheme           string      `json:"scheme,omitempty"`
    BearerFormat     string      `json:"bearerFormat,omitempty"`
    Flows            *OAuthFlows `json:"flows,omitempty"`
    OpenIDConnectURL string      `json:"openIdConnectUrl,omitempty"`
}

type OAuthFlows struct {
    Implicit          *OAuthFlow `json:"implicit,omitempty"`
    Password          *OAuthFlow `json:"password,omitempty"`
    ClientCredentials *OAuthFlow `json:"clientCredentials,omitempty"`
    AuthorizationCode *OAuthFlow `json:"authorizationCode,omitempty"`
}

type OAuthFlow struct {
    AuthorizationURL string            `json:"authorizationUrl,omitempty"`
    TokenURL         string            `json:"tokenUrl,omitempty"`
    RefreshURL       string            `json:"refreshUrl,omitempty"`
    Scopes           map[string]string `json:"scopes"`
}
```

## Builder API

`Builder` 负责把较低层的 OpenAPI 结构组织成稳定文档。它不注册 HTTP 路由，也不持有任何 server 对象。

```go
type Builder struct {
    // 内部保存 document 和 schema registry。
}

func New(cfg Config) *Builder
func NewFromDocument(doc *Document) (*Builder, error)
func (b *Builder) Document() *Document
func (b *Builder) DocumentUnsafe() *Document
func (b *Builder) JSON() ([]byte, error)
func (b *Builder) Validate() ValidationErrors
```

`Config`：

```go
type Config struct {
    OpenAPI     string
    Info        Info
    Servers     []Server
    Tags        []Tag
    TagResolvers []TagResolver
}
```

基础构建 API：

```go
func (b *Builder) AddServer(server Server) *Builder
func (b *Builder) AddTag(tag Tag) *Builder
func (b *Builder) AddSecurityScheme(name string, scheme SecurityScheme) *Builder
func (b *Builder) AddSchema(name string, schema *Schema) (SchemaRef, error)
func (b *Builder) SchemaOf(value any) (SchemaRef, error)
func SchemaOf[T any](b *Builder) (SchemaRef, error)
```

`Builder` 内部需要用锁保护，但并发模型必须简单明确：

- 所有对 Builder 状态的写操作使用同一把粗粒度互斥锁串行化。
- `SchemaOf` 在进入 schema 生成前获取锁，整个递归生成过程都在同一个临界区内完成，内部递归函数不再重复加锁。
- schema registry 和递归生成状态只在该临界区内读写，避免并发递归生成同一类型导致状态错乱。
- `Document()` 返回深拷贝，避免调用方修改内部状态。
- `DocumentUnsafe()` 是兼容旧调用的低层接口，返回内部只读引用，只用于序列化或调试；调用方不得修改返回值。

`DocumentUnsafe()` 适用场景必须非常有限：

1. `JSON()`、`Validate()` 等内部只读流程需要避免深拷贝。
2. 测试或调试时临时查看内部状态。
3. 性能敏感的只读访问，并且调用方能保证不修改返回对象。

除上述场景外，公开调用应优先使用 `Document()`。

`DocumentUnsafe()` 保留为 deprecated API；新增调用应优先使用 `Document()`、`JSON()` 或 `Validate()`，避免在锁释放后持有内部可变指针。

`NewFromDocument` 用于从已有 OpenAPI 文档继续构建。它会深拷贝输入文档、初始化 schema registry，并执行基础结构校验；输入文档不会被 Builder 修改。operationId 唯一性通过注册和校验时扫描已有 operation 保证。

## Operation API

操作注册是纯文档行为：

```go
func (b *Builder) Operation(method, path string, op Operation) error
func (b *Builder) MustOperation(method, path string, op Operation) *Builder
```

`OperationBuilder` 是仅用于组装 `Operation` 的临时对象：

```go
type OperationBuilder struct {
    // 内部保存 Operation 值。
}

func NewOperation() *OperationBuilder
func (b *OperationBuilder) OperationID(id string) *OperationBuilder
func (b *OperationBuilder) Summary(text string) *OperationBuilder
func (b *OperationBuilder) Description(text string) *OperationBuilder
func (b *OperationBuilder) Tag(name string) *OperationBuilder
func (b *OperationBuilder) Group(name string) *OperationBuilder
func (b *OperationBuilder) Groups(names ...string) *OperationBuilder
func (b *OperationBuilder) Parameter(parameter Parameter) *OperationBuilder
func (b *OperationBuilder) ParameterRef(parameter ParameterRef) *OperationBuilder
func (b *OperationBuilder) RequestBody(body RequestBody) *OperationBuilder
func (b *OperationBuilder) RequestBodyRef(body RequestBodyRef) *OperationBuilder
func (b *OperationBuilder) RequestJSON(schema SchemaRef, opts ...RequestBodyOption) *OperationBuilder
func (b *OperationBuilder) Response(status int, response Response) *OperationBuilder
func (b *OperationBuilder) ResponseRef(status int, response ResponseRef) *OperationBuilder
func (b *OperationBuilder) ResponseJSON(status int, description string, schema SchemaRef) *OperationBuilder
func (b *OperationBuilder) Deprecated() *OperationBuilder
func (b *OperationBuilder) Security(requirement SecurityRequirement) *OperationBuilder
func (b *OperationBuilder) Server(server Server) *OperationBuilder
func (b *OperationBuilder) Build() Operation
```

`Build()` 返回 `Operation` 值类型，避免调用方在注册后继续修改内部指针状态。`Operation()` 注册时会复制该值到 `Paths` 中。

分组会写入 `Operation.Groups`，JSON 输出为 `x-groups`。如果 operation 没有显式设置 `tags`，构建和注册时会默认使用最后一级 group 作为 tag，便于 UI 分组和 OpenAPI 标准 tag 同时可用。

```go
users := doc.Group("业务接口").Group("用户")

users.NewOperation("POST", "/users").
    OperationID("createUser").
    Summary("Create user").
    RequestJSON(createUserSchema).
    ResponseJSON(201, "Created", userSchema).
    MustRegister()
```

分组上下文 API：

```go
type GroupBuilder struct {
    // 内部保存 Builder 和当前分组链。
}

func (b *Builder) Group(name string) *GroupBuilder
func (g *GroupBuilder) Group(name string) *GroupBuilder
func (g *GroupBuilder) Operation(method, path string, op Operation) error
func (g *GroupBuilder) MustOperation(method, path string, op Operation) *GroupBuilder
func (g *GroupBuilder) NewOperation(method, path string) *GroupedOperationBuilder
```

`GroupedOperationBuilder` 代理 `OperationBuilder` 的常用链式方法，并提供 `Build()`、`Register()`、`MustRegister()`。`Build()` 会合并上层分组上下文，`Register()` 会把最终 operation 注册到原始 `Builder`。

推荐提供辅助构建器降低样板代码：

```go
createUserSchema, err := openapi.SchemaOf[CreateUserRequest](doc)
if err != nil {
    return err
}
userSchema, err := openapi.SchemaOf[UserResponse](doc)
if err != nil {
    return err
}
errorSchema, err := openapi.SchemaOf[ErrorResponse](doc)
if err != nil {
    return err
}

op := openapi.NewOperation().
    Summary("Create user").
    Tag("Users").
    RequestJSON(createUserSchema).
    ResponseJSON(200, "OK", userSchema).
    ResponseJSON(400, "Bad Request", errorSchema)

if err := doc.Operation("POST", "/users", op.Build()); err != nil {
    return err
}
```

`method` 必须是 OpenAPI 支持的 HTTP method，小写或大写都接受，输出统一为小写字段名。`path` 必须是 OpenAPI 路径模板，例如 `/users/{id}`。本包不接收 gin 风格 `:id` 或 `*filepath`，这类转换应由上层适配包处理。

`Operation()` 在注册时做快速校验并返回错误：

- HTTP method 是否支持。
- path 是否以 `/` 开头。
- path template 语法是否合法。
- path parameter 是否都有对应的 required parameter。
- operationId 是否和已注册 operation 冲突。

`MustOperation()` 是启动期便利方法，内部调用 `Operation()`，遇到错误直接 panic。

### Parameters

```go
func Query(name string, schema SchemaRef, opts ...ParameterOption) Parameter
func Path(name string, schema SchemaRef, opts ...ParameterOption) Parameter
func HeaderParam(name string, schema SchemaRef, opts ...ParameterOption) Parameter
func Cookie(name string, schema SchemaRef, opts ...ParameterOption) Parameter
```

path parameter 永远是 required；如果调用方显式设置 false，构建时应返回校验错误。

Parameter 构造函数保持轻量，不返回 error；空 name、无效 `SchemaRef`、非法 `in`、`schema` 和 `content` 同时为空或同时设置、path parameter 非 required 等问题统一在 `Operation()` 快速校验和 `Validate()` 批量校验中发现。这样常见链式构建不会被大量局部 error 打断。

### Request Body

```go
func JSONBody(schema SchemaRef, opts ...RequestBodyOption) RequestBody
func FormBody(schema SchemaRef, opts ...RequestBodyOption) RequestBody
func MultipartBody(schema SchemaRef, opts ...RequestBodyOption) RequestBody
func WithRequestBodyDescription(description string) RequestBodyOption
func WithRequestBodyRequired(required bool) RequestBodyOption
```

第一版至少支持：

- `application/json`
- `application/x-www-form-urlencoded`
- `multipart/form-data`

### Responses

```go
func JSONResponse(status int, description string, schema SchemaRef, opts ...ResponseOption) Response
func EmptyResponse(status int, description string, opts ...ResponseOption) Response
```

本包不假设任何业务统一响应格式。调用方需要什么响应 envelope，就显式定义对应 schema。

## Schema 生成

### 类型映射

- `string` -> `type: string`
- `bool` -> `type: boolean`
- `int`, `int8`, `int16`, `int32` -> `type: integer`, `format: int32`
- `int64` -> `type: integer`, `format: int64`
- `uint`, `uint8`, `uint16`, `uint32` -> `type: integer`, `format: int32`, `minimum: 0`
- `uint64` -> `type: integer`, `format: int64`, `minimum: 0`
- `float32` -> `type: number`, `format: float`
- `float64` -> `type: number`, `format: double`
- `time.Time` -> `type: string`, `format: date-time`
- `[]byte` -> `type: string`, `format: byte`
- slice/array -> `type: array`
- map -> `type: object`, `additionalProperties`
- struct -> `type: object`, `properties`
- pointer -> 展开底层类型，并设置 `nullable: true`

### Tag 规则

`pkg/openapi` 是独立包，不能把 gin、validator、jsoniter 等框架 tag 规则写死到核心逻辑里。Schema 生成使用可配置 tag 解析链。

默认解析器：

1. 跳过未导出字段。
2. `json` tag：处理字段名、`-` 忽略字段。
3. fallback：字段名首字母小写，例如 `CreatedAt` -> `createdAt`。
4. `openapi` tag：处理 `name=xxx`、`required`、`deprecated`、`nullable`、`desc=...`、`description=...` 等 OpenAPI 专用元数据，`name=xxx` 可以覆盖前面得到的字段名。
5. `example` tag：生成 example。
6. `default` tag：生成 default。

```go
type TagResolver interface {
    Resolve(field reflect.StructField, meta *FieldMeta) error
}

type TagResolverFunc func(field reflect.StructField, meta *FieldMeta) error

type FieldMeta struct {
    Name        string
    Ignore      bool
    Required    *bool
    Nullable    *bool
    Deprecated  bool
    Description string
    Example     any
    Default     any
    Constraints FieldConstraints
    Extensions  map[string]any
}

type FieldConstraints struct {
    Minimum   *float64
    Maximum   *float64
    MinLength *int
    MaxLength *int
    MinItems  *int
    MaxItems  *int
    Pattern   string
    Enum      []any
    Format    string
}
```

解析顺序和覆盖规则：

1. resolvers 按顺序执行，后执行的 resolver 可以补充或覆盖前一个 resolver 设置的字段。
2. `FieldMeta.Ignore=true` 后停止后续字段级解析，字段不进入 schema。
3. `Required`、`Nullable` 使用指针表示“未声明”；只有显式设置的 resolver 才覆盖值。
4. 默认 resolver 中 `openapi:"name=xxx"` 会覆盖 `json` 字段名。
5. `json:",omitempty"` 不影响 required 判断；required 表示输入文档约束，不等同于 Go JSON 序列化行为。
6. `example` 和 `default` 默认只解析基础类型：string、bool、整数、浮点数；复杂对象需要调用方手动设置 schema。
7. `Constraints` 由 schema 生成器按字段类型落到对应 schema 字段，例如字符串使用 `minLength/maxLength`，数字使用 `minimum/maximum`，slice 使用 `minItems/maxItems`。

框架适配通过自定义 resolver 实现；gin 的 `binding` 只是一个示例，同样的机制也可以支持其他框架、校验器或团队自定义 tag，例如 `validate`、`rules`、`doc`。

```go
type RuleTagResolverConfig struct {
    TagName string
    Strict  bool
}

func RuleTagResolver(cfg RuleTagResolverConfig) openapi.TagResolverFunc {
    return func(field reflect.StructField, meta *openapi.FieldMeta) error {
        value := field.Tag.Get(cfg.TagName)
        return applyRuleTag(field, value, meta, cfg.Strict)
    }
}

func applyRuleTag(field reflect.StructField, value string, meta *openapi.FieldMeta, strict bool) error {
    // 解析逗号分隔 tag 选项，例如：
    // "required,min=1,max=100"
    // "required,email"
    // "desc=用户名,min=2,max=32"
    // "oneof=admin user guest"
}
```

使用 gin binding tag：

```go
doc := openapi.New(openapi.Config{
    Info: openapi.Info{Title: "API", Version: "1.0.0"},
    TagResolvers: []openapi.TagResolver{
        openapi.DefaultTagResolver(),
        RuleTagResolver(RuleTagResolverConfig{TagName: "binding"}),
    },
})
```

使用其他校验 tag：

```go
doc := openapi.New(openapi.Config{
    Info: openapi.Info{Title: "API", Version: "1.0.0"},
    TagResolvers: []openapi.TagResolver{
        openapi.DefaultTagResolver(),
        RuleTagResolver(RuleTagResolverConfig{TagName: "validate"}),
    },
})
```

建议通用 rule resolver 支持这些常见规则：

- `required` -> `FieldMeta.Required=true`
- `desc=...` / `description=...` -> `FieldMeta.Description`
- `min=n` -> 字符串 `minLength`，数组 `minItems`，数字 `minimum`
- `max=n` -> 字符串 `maxLength`，数组 `maxItems`，数字 `maximum`
- `len=n` -> 字符串 `minLength=maxLength=n`，数组 `minItems=maxItems=n`
- `oneof=a b c` -> `enum`
- `email` -> `format: email`
- `url` / `uri` -> `format: uri`
- `uuid` -> `format: uuid`
- `datetime` -> `format: date-time`

`min`、`max`、`len` 需要基于 `reflect.StructField.Type` 判断字段类别，再落到 `FieldConstraints` 的字符串、数组或数字约束上。指针字段先解引用后判断类型。

无法识别的规则默认忽略；如果 `RuleTagResolverConfig.Strict=true`，遇到未知规则或非法数值时返回错误。

如果团队的 tag 语义更复杂，可以直接实现完整 resolver：

```go
func TeamRuleResolver(field reflect.StructField, meta *openapi.FieldMeta) error {
    if field.Tag.Get("doc") == "required" && meta.Required == nil {
        required := true
        meta.Required = &required
    }
    if desc := field.Tag.Get("desc"); desc != "" && meta.Description == "" {
        meta.Description = desc
    }
    return nil
}
```

`pkg/openapi` 核心包只提供 resolver 接口和默认 resolver，不直接依赖任何框架。后续如果需要，可以在独立 compat 子包中提供常见框架的 resolver。

### 组件命名和复用

schema 生成器维护类型注册表，避免同一个 Go 类型重复生成，也避免递归类型无限展开。

组件命名规则：

1. 命名类型默认使用 `PkgName_TypeName`，例如 `user_CreateUserRequest`。
2. 匿名结构体默认内联，不进入 `components.schemas`。
3. 不同包出现同名组件冲突时追加短 hash，例如 `user_CreateUserRequest_a1b2c3`。
4. 指针、slice、array、map 不单独命名，递归处理元素类型。

短 hash 使用完整包路径加类型名计算 FNV-1a 32-bit，取 6 位小写十六进制。相同类型在不同机器和不同运行时必须得到稳定名称。

递归处理规则：

1. 开始生成命名类型前先写入 registry，状态标记为 `building`。
2. 生成过程中再次遇到同一命名类型时直接返回 `$ref`。
3. 生成完成后移除当前递归标记；已写入 components 的 schema 后续会直接复用。

## Reference Object 策略

OpenAPI 3.0 中很多对象位置都允许使用 Reference Object，例如 schema、response、parameter、request body、header、example、link、callback 和 security scheme。模型层使用一组具体 `XxxRef` 类型表达“引用或内联对象”。

第一版不使用泛型 type alias 暴露 `SchemaRef = Ref[Schema]` 这类 API。原因是当前 Go 工具链在该形态下容易触发编译器问题；具体类型虽然代码重复一点，但 API 稳定、文档清晰、实现风险更低。

```go
type SchemaRef struct {
    Ref    string
    Inline *Schema
}

type ResponseRef struct {
    Ref    string
    Inline *Response
}

// ParameterRef、RequestBodyRef、HeaderRef、ExampleRef、LinkRef、
// CallbackRef、SecuritySchemeRef 使用同样结构，只是 Inline 类型不同。
```

建议提供引用构造函数，避免调用方直接拼结构体字段：

```go
func SchemaReference(ref string) SchemaRef
func SchemaInline(schema *Schema) SchemaRef
func ResponseReference(ref string) ResponseRef
func ResponseInline(response *Response) ResponseRef
```

每个 `XxxRef` 必须实现自定义 JSON 序列化和反序列化，内部可以复用泛型私有 helper 降低重复：

```go
func marshalRef[T any](ref string, inline *T) ([]byte, error) {
    switch {
    case ref != "" && inline == nil:
        return json.Marshal(map[string]string{"$ref": ref})
    case ref == "" && inline != nil:
        return json.Marshal(inline)
    case ref == "" && inline == nil:
        return nil, errors.New("openapi: empty reference")
    default:
        return nil, errors.New("openapi: reference cannot contain both Ref and Inline")
    }
}

func unmarshalRef[T any](data []byte, refValue *string, inline **T) error {
    var refObject struct {
        Ref string `json:"$ref"`
    }
    if err := json.Unmarshal(data, &refObject); err != nil {
        return err
    }
    if refObject.Ref != "" {
        *refValue = refObject.Ref
        *inline = nil
        return nil
    }

    var value T
    if err := json.Unmarshal(data, &value); err != nil {
        return err
    }
    *refValue = ""
    *inline = &value
    return nil
}
```

规则：

- `Ref` 非空时表示 components 或远程引用，序列化时只输出 `$ref`。
- `Inline` 非空时表示内联对象，序列化时输出完整对象。
- `Ref` 和 `Inline` 不能同时为空，也不能同时非空；构造函数和 `Validate()` 都要检查。
- `Validate()` 需要检查所有本地 `$ref` 是否能在 `components` 中找到，包括 schema、response、parameter、request body、header、example、link、callback 和 security scheme。
- 第一版只保证本地引用，远程 URL 引用保留为原始字符串，不做网络校验。
- 可选字段必须使用 `*XxxRef`，例如 `MediaType.Schema *SchemaRef`、`Parameter.Schema *SchemaRef`。如果使用值类型配合 `omitempty`，空引用会触发 `MarshalJSON()` 错误，不能正确省略字段。
- `PathItem` 的 `$ref` 是 OpenAPI Path Item Object 自身字段，直接用 `PathItem.Ref` 表达。`Validate()` 应禁止同一个 `PathItem` 同时设置 `Ref` 和 operation 字段，避免产生规范中的未定义行为。

`additionalProperties` 是 OpenAPI 3.0 的特殊字段，既可以是 boolean，也可以是 schema，不能复用普通 `SchemaRef`：

```go
type AdditionalProperties struct {
    Allowed *bool
    Schema  *SchemaRef
}
```

序列化规则：

- `Allowed` 非空时输出 JSON boolean。
- `Schema` 非空时输出 schema 或 `$ref`。
- 两者不能同时为空，也不能同时非空。

## 校验

`Validate()` 返回结构化错误列表，而不是只返回第一个错误：

```go
type ValidationError struct {
    Location string
    Message  string
}

type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string
func (e ValidationErrors) HasErrors() bool
```

`Location` 使用稳定路径描述错误位置，例如 `paths./users/{id}.get.parameters[0]` 或 `components.schemas.User.properties.id`。

`Validate()` 第一版检查：

- `openapi` 版本非空且支持 `3.0.3`。
- `info.title` 和 `info.version` 非空。
- `paths` 非空。
- path 必须以 `/` 开头，path template 参数形如 `{id}`。
- path parameter 必须在 path template 中存在。
- path template 中的参数必须有对应 required path parameter。
- PathItem 不能同时设置 `$ref` 和 operation 字段。
- Parameter 和 Header 必须且只能设置 `schema` 或 `content` 之一。
- operationId 不重复。
- response status code 合法，允许 `default`。
- 本地 `$ref` 可解析，覆盖 schema、response、parameter、request body、header、example、link、callback、security scheme。

校验只覆盖文档结构正确性，不做业务语义判断。

快速失败和批量校验的边界：

- `Operation()` 对 method、path、path parameter、operationId 做快速校验，方便启动期尽早失败。
- `Validate()` 对整份文档做批量校验，返回所有可发现问题。

## 错误处理

- 普通构建 API 返回 `error` 或 `ValidationErrors`，不使用 panic。
- `MustOperation` 这类 `Must` 前缀方法可以 panic，只用于启动期常量式配置。
- schema 生成遇到不支持类型，例如 `chan`、`func`、`unsafe.Pointer`，返回明确错误并带上 Go 类型名。
- `SchemaOf` 解析 tag 失败时返回错误，例如非法 openapi tag、无法转换的 example/default。
- UI 加载 spec 失败时在页面内展示错误状态，包括 HTTP 状态码、请求 URL 和简短错误信息；不展示堆栈。

## 序列化

第一版必须支持 JSON 的读取和写出：

```go
func Marshal(doc *Document) ([]byte, error)
func MarshalIndent(doc *Document, prefix, indent string) ([]byte, error)
func Unmarshal(data []byte) (*Document, error)
func ReadFile(path string) (*Document, error)
```

`Unmarshal` 只负责解析 JSON 到 `Document`，并保证各类 `XxxRef` 能正确解析 `$ref` 和内联对象，`AdditionalProperties` 能正确解析 boolean 和 schema。结构合法性由 `Validate()` 负责。`ReadFile` 是便利函数，只读取本地 JSON 文件，不做远程 URL 拉取。

导入限制：

- `Unmarshal` 负责读取 JSON 并保留文档内容，不做版本校验。
- 如果 `openapi` 版本为空或不是 `3.0.3`，`Validate()` 返回错误。
- 远程 `$ref` 保留字符串，不下载、不解析。
- YAML 可后续增加。为了避免引入额外依赖，第一版不强制支持 YAML。

## 版本兼容

第一版只输出 OpenAPI 3.0.3。为了后续支持 OpenAPI 3.1，设计上需要遵守：

- `Document.OpenAPI` 由 `Config.OpenAPI` 控制，当前校验只接受 `3.0.3`。
- Schema Object 内部保留 OpenAPI 3.0 的 `nullable` 语义，不提前引入 3.1 的 JSON Schema type union。
- 后续新增 3.1 时优先通过新的 schema 转换选项实现，不破坏 3.0 默认输出。

## 性能目标

第一版性能目标用于指导实现和测试，不作为严格 SLA：

- 生成 1000 个普通 struct schema 应在 1 秒内完成。
- 生成 1000 个普通 struct schema 的额外内存占用应小于 50MB。
- 序列化 10MB OpenAPI JSON 应在 500ms 内完成。
- 序列化 10MB OpenAPI JSON 的峰值额外内存应小于 30MB。
- UI 加载 1000 个 operation 的文档时首屏可交互时间应控制在 2 秒内。
- UI 搜索和展开操作应避免全量重排，优先使用索引和局部渲染。

## 示例

### 端到端构建

```go
type CreateUserRequest struct {
    Name  string `json:"name" openapi:"required" example:"Alice"`
    Email string `json:"email" openapi:"required" example:"alice@example.com"`
}

type UserResponse struct {
    ID    int64  `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

doc := openapi.New(openapi.Config{
    Info: openapi.Info{
        Title:   "User API",
        Version: "1.0.0",
    },
})

reqSchema, err := openapi.SchemaOf[CreateUserRequest](doc)
if err != nil {
    return err
}
respSchema, err := openapi.SchemaOf[UserResponse](doc)
if err != nil {
    return err
}

op := openapi.NewOperation().
    OperationID("createUser").
    Summary("Create user").
    Tag("Users").
    RequestJSON(reqSchema).
    ResponseJSON(200, "OK", respSchema)

if err := doc.Operation("POST", "/users", op.Build()); err != nil {
    return err
}
if errs := doc.Validate(); errs.HasErrors() {
    return errs
}

payload, err := doc.JSON()
```

### 递归类型

```go
type Category struct {
    ID       int64      `json:"id"`
    Name     string     `json:"name"`
    Children []Category `json:"children"`
}
```

`SchemaOf[Category]` 应只生成一个 `Category` component；`children.items` 使用 `$ref` 指回该 component。

## UI

`pkg/openapi/ui` 自研前端页面，使用 embed 打包进 Go 二进制。UI 信息架构可以参考 Apifox：左侧接口目录，中间接口详情，右侧辅助信息，整体偏工程工具风格，优先保证高密度、可搜索、可快速定位。

第一版页面能力：

- 加载 OpenAPI 3 JSON 文档。
- 支持通过 `SpecURL` 加载已有 OpenAPI JSON 文件或接口返回的 JSON。
- 左侧接口分组、路径搜索、HTTP method 颜色标识。
- 主区域展示接口摘要、描述、参数表、请求体 schema、响应 schema、示例响应。
- 顶部展示文档标题、版本、server 切换、JSON 文档入口。
- 右侧展示当前接口目录锚点，例如 Parameters、Request Body、Responses、Schemas。
- 支持展开/折叠 operation。
- 支持 `x-groups` 多级分组，未设置时从 tag 或 path 推导默认分组。
- 支持 schema 树形视图。
- 支持复制 JSON schema 和 curl 样例。
- 支持 Try It 请求调试、全局 header、请求 header 快捷编辑和响应查看。
- 支持浅色/深色主题，主题状态保存在 localStorage。

Try It 通过 Go UI 代理发起请求。默认仅允许当前请求 host、`localhost`、`127.0.0.0/8` 和 `::1` 目标；非本机目标需要调用方显式配置 `ProxyHosts`，生产环境可以通过 `DisableProxy` 关闭。

### UI Handler

UI 包只暴露标准库 handler，不绑定具体 server 框架：

```go
type Config struct {
    Title                 string
    SpecURL               string
    CSP                   string
    CORS                  *CORSConfig
    StoragePrefix         string
    DisableProxy          bool
    ProxyHosts            []string
    MaxProxyRequestBytes  int64
    MaxProxyResponseBytes int64
    MaxProxyFileBytes     int64
}

type CORSConfig struct {
    AllowedOrigins []string
    AllowedHeaders []string
}

func Handler(cfg Config) http.Handler
func SpecHandler(doc *openapi.Document) http.Handler
func FileSpecHandler(path string) http.Handler
```

调用方可以把 handler 接入任意 HTTP 框架。`pkg/openapi` 不负责接入方式。

`FileSpecHandler` 用于直接暴露本地 OpenAPI JSON 文件。它在每次请求时读取文件，便于开发期更新；生产场景可以由调用方自行缓存或使用 `SpecHandler` 暴露内存中的 `Document`。

安全默认值：

- 默认不设置 CORS；同源加载 spec 是推荐路径。
- 如果 `CORS` 非空，handler 只按配置输出最小 CORS 响应头。
- 默认 CSP 使用自包含静态资源策略，不允许远程脚本。
- 默认设置 `X-Frame-Options: DENY`，避免文档 UI 被嵌入 iframe。
- 默认设置 `X-Content-Type-Options: nosniff`。
- 默认请求代理只允许同源和本机地址；跨主机代理必须通过 `ProxyHosts` 显式放行。
- 默认代理限制为请求体 64MiB、响应体 32MiB、单文件 32MiB，可通过 `MaxProxyRequestBytes`、`MaxProxyResponseBytes`、`MaxProxyFileBytes` 调整。
- 生产环境不需要 Try It 时应设置 `DisableProxy`，避免 UI 被用作内网请求跳板。

主题策略：

- 默认跟随系统主题。
- 页面提供显式主题切换按钮。
- localStorage key 默认使用 `openapi-ui-theme`。
- 如果 `StoragePrefix` 非空，localStorage key 使用 `<StoragePrefix>-theme`。

静态资源 handler 需要处理：

- `Content-Type`：根据扩展名或 `http.DetectContentType` 设置。
- `Cache-Control`：当前 HTML 和静态资源默认 `no-store`，避免开发期 UI 和文档配置缓存造成干扰。
- 404：资源不存在时返回 `404`，不输出目录列表。

## 实施阶段

### Phase 1: OpenAPI 3 文档模型

- 定义 OpenAPI 3.0.3 结构体。
- 实现 JSON 序列化和反序列化。
- 实现结构化 `Validate()`。

交付物：

- `document.go`
- `ref.go`
- `encoding.go`
- `validate.go`

验收标准：

- 所有第一版核心结构体字段定义完整。
- `XxxRef.MarshalJSON()` 能正确输出 `$ref` 或内联对象。
- `XxxRef.UnmarshalJSON()` 能正确读取 `$ref` 和内联对象。
- `AdditionalProperties` 能正确读写 boolean 和 schema 两种形态。
- JSON 输出能覆盖 OpenAPI 3.0.3 官方基础示例。
- `Validate()` 能检测缺失 info、非法 path、path parameter 不匹配、operationId 重复、无效 `$ref`。
- Phase 1 相关单元测试通过。

### Phase 2: Builder 和 Schema

- 实现 `Builder`。
- 实现 `Operation` 辅助构建器。
- 实现 Go 类型到 Schema 的转换。
- 实现 components registry、命名冲突处理和递归引用。
- 实现 `XxxRef` 序列化、反序列化和 `$ref` 校验。
- 实现默认 tag resolver 和自定义 resolver 链。

交付物：

- `builder.go`
- `operation.go`
- `schema.go`
- tag 解析相关实现

验收标准：

- `Operation()` 能快速校验 method、path、path parameter、operationId。
- `SchemaOf` 支持基础类型、struct、pointer、slice、map、匿名结构体。
- 自定义 `TagResolver` 可以补充 required、description、nullable、deprecated、example、default 和常见约束等字段元数据。
- 递归类型生成稳定 `$ref`，不会无限展开。
- 同名类型冲突使用稳定 hash 命名。
- 不支持类型和非法 tag 返回明确错误。
- Phase 2 相关单元测试通过。

### Phase 3: UI

- 在 `openapi/ui` 下实现前端页面。
- 用 embed 打包静态资源。
- 实现 `ui.Handler`、`ui.SpecHandler` 和 `ui.FileSpecHandler`。

交付物：

- `ui/embed.go`
- `ui/handler.go`
- `ui/index.html`
- `ui/app.css`
- `ui/app.js`

验收标准：

- UI 能加载并展示 OpenAPI 3 JSON。
- UI 能通过 `SpecURL` 加载外部 OpenAPI JSON。
- 支持搜索、接口详情、schema 树、响应展示和主题切换。
- handler 输出正确 Content-Type、CSP、X-Frame-Options、X-Content-Type-Options。
- 静态资源不存在时返回 404，不输出目录列表。
- Phase 3 相关单元测试和基础浏览器验证通过。

### Phase 4: 增强能力

- 支持 OpenAPI 3.1。
- 支持 YAML 序列化。
- 支持 examples 管理。
- 支持更多 JSON Schema 关键字。
- 支持 Try It 请求调试。
- 支持文档导入和 diff。

## 测试策略

- 文档模型测试覆盖 JSON 字段名和 omitempty 行为。
- JSON 导入测试覆盖 `$ref` 对象、内联对象、`additionalProperties` boolean/schema、非法 JSON、版本不支持。
- Builder 测试覆盖 paths、operations、components、security schemes、tags。
- schema 生成表驱动测试覆盖基础类型、struct、pointer、slice、map、默认 tag、自定义 tag resolver、匿名结构体。
- schema registry 测试覆盖同名类型冲突、递归类型、重复引用复用。
- Ref 测试覆盖 `$ref` 输出、内联输出、空引用、双重引用错误。
- Validate 测试覆盖缺失 info、非法 path、path parameter 不匹配、operationId 重复、无效 `$ref` 和多错误返回。
- 错误处理测试覆盖不支持 Go 类型、非法 tag、example/default 转换失败。
- UI handler 测试覆盖 HTML 返回、静态资源 content type、资源 404、spec JSON 返回、文件 spec 返回、安全响应头。

## 风险和约束

- OpenAPI 3.0 Schema Object 和完整 JSON Schema 不完全一致，不能直接照搬 OpenAPI 3.1 语义。
- Go 类型到 schema 的转换无法覆盖全部业务语义，复杂约束应由调用方显式补充。
- 第一版不和 server 包关联，因此不会自动知道真实运行时路由；路径和 operation 需要调用方显式声明。
- Builder 使用粗粒度锁保证实现简单，初始化期并发安全优先于极致吞吐。
- `DocumentUnsafe()` 有误用风险，文档和注释必须明确只读约束。
- UI 参考 Apifox 的信息架构，不复制其品牌、样式资产或专有实现。
