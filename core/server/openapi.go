package server

import (
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/surge-go/fox/pkg/openapi"
)

const (
	bindingTypeNone  = ""
	bindingTypeJSON  = "json"
	bindingTypeQuery = "query"
	bindingTypeURI   = "uri"
	bindingTypeAuto  = "auto"
)

var (
	handlerTypeMap  sync.Map
	colonParamRegex = regexp.MustCompile(`:([a-zA-Z_][a-zA-Z0-9_]*)`)
	starParamRegex  = regexp.MustCompile(`\*([a-zA-Z_][a-zA-Z0-9_]*)`)
)

type handlerTypeInfo struct {
	reqType     reflect.Type
	respType    reflect.Type
	bindingType string
	handlerName string
	noRequest   bool
	noResponse  bool
}

// Doc 表示单个路由的 OpenAPI 文档元数据。
type Doc struct {
	Summary     string
	Description string
	Tags        []string
	OperationID string
}

// DocGroup 表示 RouterGroup 继承给路由的 OpenAPI UI 分组。
type DocGroup struct {
	Groups []string
}

// Route 表示已注册路由的文档配置入口。
type Route struct {
	mu         sync.RWMutex
	engine     *Engine
	method     string
	path       string
	handler    HandlerFunc
	doc        *Doc
	deprecated bool
	security   []string
	docGroups  []string
}

type routeSnapshot struct {
	method     string
	path       string
	handler    HandlerFunc
	doc        *Doc
	deprecated bool
	security   []string
	docGroups  []string
}

func typeOf[T any]() reflect.Type {
	return reflect.TypeOf((*T)(nil)).Elem()
}

func registerHandlerType(fn HandlerFunc, info *handlerTypeInfo) HandlerFunc {
	handlerTypeMap.Store(reflect.ValueOf(fn).Pointer(), info)
	return fn
}

func extractTypeInfoFromHandler(fn HandlerFunc) *handlerTypeInfo {
	if info, ok := handlerTypeMap.Load(reflect.ValueOf(fn).Pointer()); ok {
		if typed, ok := info.(*handlerTypeInfo); ok {
			return typed
		}
	}
	return nil
}

func newRoute(engine *Engine, method, path string, handler HandlerFunc, groups []string) *Route {
	return &Route{
		engine:    engine,
		method:    method,
		path:      path,
		handler:   handler,
		docGroups: cloneStrings(groups),
	}
}

// Doc 覆盖当前路由的文档元数据。
func (r *Route) Doc(doc Doc) *Route {
	if r == nil {
		return r
	}
	doc.Tags = cloneStrings(doc.Tags)
	r.mu.Lock()
	r.doc = &doc
	r.mu.Unlock()
	r.markForDoc()
	return r
}

// Summary 配置 OpenAPI operation summary。
func (r *Route) Summary(summary string) *Route {
	if r == nil {
		return r
	}
	r.mu.Lock()
	r.ensureDocLocked().Summary = summary
	r.mu.Unlock()
	r.markForDoc()
	return r
}

// Description 配置 OpenAPI operation description。
func (r *Route) Description(desc string) *Route {
	if r == nil {
		return r
	}
	r.mu.Lock()
	r.ensureDocLocked().Description = desc
	r.mu.Unlock()
	r.markForDoc()
	return r
}

// OperationID 配置 OpenAPI operationId。
func (r *Route) OperationID(id string) *Route {
	if r == nil {
		return r
	}
	r.mu.Lock()
	r.ensureDocLocked().OperationID = id
	r.mu.Unlock()
	r.markForDoc()
	return r
}

// Tags 追加 OpenAPI operation tags。
func (r *Route) Tags(tags ...string) *Route {
	if r == nil {
		return r
	}
	r.mu.Lock()
	doc := r.ensureDocLocked()
	doc.Tags = append(doc.Tags, tags...)
	r.mu.Unlock()
	r.markForDoc()
	return r
}

// Tag 追加单个 OpenAPI operation tag。
func (r *Route) Tag(tag string) *Route {
	return r.Tags(tag)
}

// Groups 追加 OpenAPI x-groups，用于 UI 多级分组。
func (r *Route) Groups(groups ...string) *Route {
	if r == nil {
		return r
	}
	r.mu.Lock()
	r.docGroups = append(r.docGroups, groups...)
	r.mu.Unlock()
	r.markForDoc()
	return r
}

// Deprecated 标记 OpenAPI operation deprecated。
func (r *Route) Deprecated() *Route {
	if r == nil {
		return r
	}
	r.mu.Lock()
	r.deprecated = true
	r.mu.Unlock()
	r.markForDoc()
	return r
}

// Security 追加 OpenAPI security requirement 名称。
func (r *Route) Security(schemes ...string) *Route {
	if r == nil {
		return r
	}
	r.mu.Lock()
	r.security = append(r.security, schemes...)
	r.mu.Unlock()
	r.markForDoc()
	return r
}

func (r *Route) ensureDocLocked() *Doc {
	if r.doc == nil {
		r.doc = &Doc{}
	}
	return r.doc
}

func (r *Route) markForDocIfDocumentable() {
	if r == nil || extractTypeInfoFromHandler(r.handler) == nil {
		return
	}
	r.markForDoc()
}

func (r *Route) markForDoc() {
	if r == nil || r.engine == nil || r.engine.cfg.OpenAPI == nil {
		return
	}
	r.engine.mu.Lock()
	defer r.engine.mu.Unlock()
	if r.engine.docRoutes == nil {
		r.engine.docRoutes = make(map[*Route]struct{})
	}
	r.engine.docRoutes[r] = struct{}{}
}

func (r *Route) snapshot() routeSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var doc *Doc
	if r.doc != nil {
		copied := *r.doc
		copied.Tags = cloneStrings(r.doc.Tags)
		doc = &copied
	}
	return routeSnapshot{
		method:     r.method,
		path:       r.path,
		handler:    r.handler,
		doc:        doc,
		deprecated: r.deprecated,
		security:   cloneStrings(r.security),
		docGroups:  cloneStrings(r.docGroups),
	}
}

// OpenAPIDocument 根据已注册的官方 wrapper 路由生成 OpenAPI 文档。
func (e *Engine) OpenAPIDocument() *openapi.Document {
	if e == nil || e.cfg == nil || e.cfg.OpenAPI == nil {
		return nil
	}
	cfg := e.cfg.OpenAPI
	builder := openapi.New(openapi.Config{
		OpenAPI:      "3.0.3",
		Info:         cfg.Info,
		Servers:      cfg.Servers,
		Tags:         cfg.Tags,
		TagResolvers: openAPITagResolvers(cfg),
	})

	e.mu.Lock()
	routes := make([]*Route, 0, len(e.docRoutes))
	for route := range e.docRoutes {
		routes = append(routes, route)
	}
	e.mu.Unlock()

	// 对路由排序以保证文档生成的稳定性
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].method != routes[j].method {
			return routes[i].method < routes[j].method
		}
		return routes[i].path < routes[j].path
	})

	for _, route := range routes {
		finalizeOpenAPIDoc(route.snapshot(), builder, cfg)
	}
	return builder.Document()
}

func openAPITagResolvers(cfg *OpenAPIConfig) []openapi.TagResolver {
	if cfg != nil && len(cfg.TagResolvers) > 0 {
		return cfg.TagResolvers
	}
	return []openapi.TagResolver{
		openapi.DefaultTagResolver(),
		openapi.RuleTagResolver(openapi.RuleTagResolverConfig{TagName: "binding"}),
		openapi.RuleTagResolver(openapi.RuleTagResolverConfig{TagName: "validate"}),
	}
}

func finalizeOpenAPIDoc(route routeSnapshot, builder *openapi.Builder, cfg *OpenAPIConfig) {
	info := extractTypeInfoFromHandler(route.handler)
	if info == nil {
		return
	}

	op := openapi.NewOperation()
	if route.doc != nil {
		op.Summary(route.doc.Summary).
			Description(route.doc.Description)
		for _, tag := range route.doc.Tags {
			op.Tag(tag)
		}
		if route.doc.OperationID != "" {
			op.OperationID(route.doc.OperationID)
		}
	}
	if len(route.docGroups) > 0 {
		op.Groups(route.docGroups...)
	}
	if route.deprecated {
		op.Deprecated()
	}
	for _, scheme := range route.security {
		op.Security(openapi.SecurityRequirement{scheme: []string{}})
	}

	addOpenAPIRequest(op, builder, info)

	// 使用配置的响应描述，支持国际化
	desc := defaultResponseDescriptions(cfg)
	op.ResponseJSON(http.StatusOK, desc.Success, responseSchema(builder, info)).
		ResponseJSON(http.StatusBadRequest, desc.BadRequest, errorResponseSchema()).
		ResponseJSON(http.StatusInternalServerError, desc.InternalServerError, errorResponseSchema())

	_ = builder.Operation(route.method, ginPathToOpenAPIPath(route.path), op.Build())
}

func defaultResponseDescriptions(cfg *OpenAPIConfig) ResponseDescriptions {
	desc := cfg.ResponseDescriptions
	if desc.Success == "" {
		desc.Success = "Success"
	}
	if desc.BadRequest == "" {
		desc.BadRequest = "Bad Request"
	}
	if desc.InternalServerError == "" {
		desc.InternalServerError = "Internal Server Error"
	}
	return desc
}

func addOpenAPIRequest(op *openapi.OperationBuilder, builder *openapi.Builder, info *handlerTypeInfo) {
	if info.noRequest || info.reqType == nil {
		return
	}
	switch info.bindingType {
	case bindingTypeQuery:
		for _, param := range structToParameters(info.reqType, "query", builder) {
			op.Parameter(param)
		}
	case bindingTypeURI:
		for _, param := range structToParameters(info.reqType, "path", builder) {
			op.Parameter(param)
		}
	case bindingTypeJSON, bindingTypeAuto:
		reqSchema, err := builder.SchemaOfType(info.reqType)
		if err != nil {
			// 记录警告但继续生成文档，使用空 schema
			reqSchema = openapi.SchemaInline(&openapi.Schema{})
		}
		op.RequestJSON(reqSchema, openapi.WithRequestBodyRequired(true))
	}
}

func responseSchema(builder *openapi.Builder, info *handlerTypeInfo) openapi.SchemaRef {
	var dataSchema openapi.SchemaRef
	if info != nil && !info.noResponse && info.respType != nil {
		var err error
		dataSchema, err = builder.SchemaOfType(info.respType)
		if err != nil {
			// 记录警告但继续生成文档，使用空 schema
			dataSchema = openapi.SchemaInline(&openapi.Schema{})
		}
	}
	return wrapResponseSchema(dataSchema, info == nil || info.noResponse || info.respType == nil)
}

func wrapResponseSchema(dataSchema openapi.SchemaRef, noResponse bool) openapi.SchemaRef {
	properties := map[string]openapi.SchemaRef{
		"code": openapi.SchemaInline(&openapi.Schema{
			Type:        "integer",
			Description: "业务状态码",
			Example:     200,
		}),
		"message": openapi.SchemaInline(&openapi.Schema{
			Type:        "string",
			Description: "响应消息",
			Example:     "success",
		}),
		"trace_id": openapi.SchemaInline(&openapi.Schema{
			Type:        "string",
			Description: "链路追踪 ID",
		}),
	}
	if noResponse {
		properties["data"] = openapi.SchemaInline(&openapi.Schema{
			Nullable:    true,
			Description: "响应数据；无业务响应时为 null",
			Example:     nil,
		})
	} else {
		properties["data"] = dataSchema
	}
	return openapi.SchemaInline(&openapi.Schema{
		Type:       "object",
		Required:   []string{"code", "message", "data"},
		Properties: properties,
	})
}

func errorResponseSchema() openapi.SchemaRef {
	return openapi.SchemaInline(&openapi.Schema{
		Type:     "object",
		Required: []string{"code", "message", "data"},
		Properties: map[string]openapi.SchemaRef{
			"code":     openapi.SchemaInline(&openapi.Schema{Type: "integer"}),
			"message":  openapi.SchemaInline(&openapi.Schema{Type: "string"}),
			"data":     openapi.SchemaInline(&openapi.Schema{Nullable: true}),
			"trace_id": openapi.SchemaInline(&openapi.Schema{Type: "string"}),
		},
	})
}

func ginPathToOpenAPIPath(path string) string {
	result := colonParamRegex.ReplaceAllString(path, "{$1}")
	return starParamRegex.ReplaceAllString(result, "{$1}")
}

func structToParameters(t reflect.Type, in string, builder *openapi.Builder) []openapi.Parameter {
	t = derefReflectType(t)
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}

	tagName := "form"
	if in == "path" {
		tagName = "uri"
	}

	params := make([]openapi.Parameter, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}

		meta, ignored := openAPIFieldMeta(field)
		if ignored {
			continue
		}

		name, skip := parameterName(field, tagName, meta)
		if skip {
			continue
		}

		schemaRef := fieldToSchema(field, builder, meta)
		param := openapi.Parameter{
			Name:        name,
			In:          in,
			Required:    in == "path" || fieldRequired(meta),
			Description: meta.Description,
			Schema:      &schemaRef,
		}
		params = append(params, param)
	}
	return params
}

func openAPIFieldMeta(field reflect.StructField) (openapi.FieldMeta, bool) {
	meta := openapi.FieldMeta{}
	resolvers := []openapi.TagResolver{
		openapi.DefaultTagResolver(),
		openapi.RuleTagResolver(openapi.RuleTagResolverConfig{TagName: "binding"}),
		openapi.RuleTagResolver(openapi.RuleTagResolverConfig{TagName: "validate"}),
	}
	for _, resolver := range resolvers {
		if err := resolver.Resolve(field, &meta); err != nil {
			continue
		}
		if meta.Ignore {
			return meta, true
		}
	}
	if meta.Description == "" {
		meta.Description = field.Tag.Get("desc")
	}
	return meta, false
}

func parameterName(field reflect.StructField, tagName string, meta openapi.FieldMeta) (string, bool) {
	if name, ignored := firstTagValue(field.Tag.Get(tagName)); ignored {
		return "", true
	} else if name != "" {
		return name, false
	}
	if meta.Name != "" {
		return meta.Name, false
	}
	return lowerFirst(field.Name), false
}

func firstTagValue(tag string) (string, bool) {
	if tag == "-" {
		return "", true
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "-" {
		return "", true
	}
	return name, false
}

func fieldToSchema(field reflect.StructField, builder *openapi.Builder, meta openapi.FieldMeta) openapi.SchemaRef {
	ref, err := builder.SchemaOfType(field.Type)
	if err != nil {
		ref = openapi.SchemaInline(&openapi.Schema{})
	}

	var schema *openapi.Schema
	if ref.Inline != nil {
		clone := *ref.Inline
		schema = &clone
	} else {
		schema = &openapi.Schema{AllOf: []openapi.SchemaRef{ref}}
	}

	if meta.Nullable != nil {
		schema.Nullable = *meta.Nullable
	}
	if meta.Deprecated {
		schema.Deprecated = true
	}
	if meta.Description != "" {
		schema.Description = meta.Description
	}
	if meta.Example != nil {
		schema.Example = meta.Example
	}
	if meta.Default != nil {
		schema.Default = meta.Default
	}
	applyOpenAPIConstraints(schema, meta.Constraints)
	return openapi.SchemaInline(schema)
}

func applyOpenAPIConstraints(schema *openapi.Schema, constraints openapi.FieldConstraints) {
	if constraints.Minimum != nil {
		schema.Minimum = constraints.Minimum
	}
	if constraints.Maximum != nil {
		schema.Maximum = constraints.Maximum
	}
	if constraints.MinLength != nil {
		schema.MinLength = constraints.MinLength
	}
	if constraints.MaxLength != nil {
		schema.MaxLength = constraints.MaxLength
	}
	if constraints.MinItems != nil {
		schema.MinItems = constraints.MinItems
	}
	if constraints.MaxItems != nil {
		schema.MaxItems = constraints.MaxItems
	}
	if constraints.Pattern != "" {
		schema.Pattern = constraints.Pattern
	}
	if len(constraints.Enum) > 0 {
		schema.Enum = constraints.Enum
	}
	if constraints.Format != "" {
		schema.Format = constraints.Format
	}
}

func fieldRequired(meta openapi.FieldMeta) bool {
	return meta.Required != nil && *meta.Required
}

func derefReflectType(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func lowerFirst(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToLower(value[:1]) + value[1:]
}
