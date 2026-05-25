package openapi

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
)

const defaultOpenAPIVersion = "3.0.3"

type Config struct {
	OpenAPI      string
	Info         Info
	Servers      []Server
	Tags         []Tag
	TagResolvers []TagResolver
}

type Builder struct {
	mu          sync.Mutex
	doc         Document
	resolvers   []TagResolver
	schemaTypes map[reflect.Type]string
	schemaNames map[string]reflect.Type
}

type GroupBuilder struct {
	builder *Builder
	groups  []string
}

type GroupedOperationBuilder struct {
	group  *GroupBuilder
	method string
	path   string
	op     OperationBuilder
}

func New(cfg Config) *Builder {
	version := cfg.OpenAPI
	if version == "" {
		version = defaultOpenAPIVersion
	}

	resolvers := cfg.TagResolvers
	if len(resolvers) == 0 {
		resolvers = []TagResolver{DefaultTagResolver()}
	}

	return &Builder{
		resolvers:   resolvers,
		schemaTypes: make(map[reflect.Type]string),
		schemaNames: make(map[string]reflect.Type),
		doc: Document{
			OpenAPI: version,
			Info:    cfg.Info,
			Servers: cfg.Servers,
			Paths:   make(Paths),
			Tags:    cfg.Tags,
			Components: &Components{
				Schemas:         make(map[string]SchemaRef),
				Responses:       make(map[string]ResponseRef),
				Parameters:      make(map[string]ParameterRef),
				RequestBodies:   make(map[string]RequestBodyRef),
				Headers:         make(map[string]HeaderRef),
				Examples:        make(map[string]ExampleRef),
				SecuritySchemes: make(map[string]SecuritySchemeRef),
				Links:           make(map[string]LinkRef),
				Callbacks:       make(map[string]CallbackRef),
			},
		},
	}
}

func NewFromDocument(doc *Document) (*Builder, error) {
	if doc == nil {
		return nil, errors.New("openapi: document is nil")
	}

	clone, err := cloneDocument(doc)
	if err != nil {
		return nil, err
	}
	if clone.Paths == nil {
		clone.Paths = make(Paths)
	}
	ensureComponents(clone)

	b := &Builder{
		doc:         *clone,
		resolvers:   []TagResolver{DefaultTagResolver()},
		schemaTypes: make(map[reflect.Type]string),
		schemaNames: make(map[string]reflect.Type),
	}
	if errs := b.Validate(); errs.HasErrors() {
		return nil, errs
	}
	return b, nil
}

func (b *Builder) AddServer(server Server) *Builder {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.doc.Servers = append(b.doc.Servers, server)
	return b
}

func (b *Builder) AddTag(tag Tag) *Builder {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.doc.Tags = append(b.doc.Tags, tag)
	return b
}

func (b *Builder) Group(name string) *GroupBuilder {
	group := strings.TrimSpace(name)
	if group == "" {
		return &GroupBuilder{builder: b}
	}
	return &GroupBuilder{builder: b, groups: []string{group}}
}

func (b *Builder) AddSecurityScheme(name string, scheme SecurityScheme) *Builder {
	b.mu.Lock()
	defer b.mu.Unlock()
	ensureComponents(&b.doc)
	b.doc.Components.SecuritySchemes[name] = SecuritySchemeRef{Inline: &scheme}
	return b
}

func (b *Builder) AddSchema(name string, schema *Schema) (SchemaRef, error) {
	if name == "" {
		return SchemaRef{}, errors.New("openapi: schema name is empty")
	}
	if schema == nil {
		return SchemaRef{}, errors.New("openapi: schema is nil")
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	ensureComponents(&b.doc)
	b.doc.Components.Schemas[name] = SchemaInline(schema)
	return SchemaReference("#/components/schemas/" + name), nil
}

func (b *Builder) Document() *Document {
	b.mu.Lock()
	defer b.mu.Unlock()
	clone, err := cloneDocument(&b.doc)
	if err != nil {
		return nil
	}
	return clone
}

// DocumentUnsafe returns the builder-owned document pointer.
//
// Deprecated: use Document, JSON, or Validate instead. The returned value is not
// protected by Builder's lock after this method returns and must not be mutated.
func (b *Builder) DocumentUnsafe() *Document {
	b.mu.Lock()
	defer b.mu.Unlock()
	return &b.doc
}

func (b *Builder) JSON() ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Marshal(&b.doc)
}

func (b *Builder) Validate() ValidationErrors {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Validate(&b.doc)
}

func (b *Builder) Operation(method, path string, op Operation) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	op = normalizeOperationGroups(op)
	if b.doc.Paths == nil {
		b.doc.Paths = make(Paths)
	}
	if err := validateOperationRegistration(&b.doc, method, path, op); err != nil {
		return err
	}

	item := b.doc.Paths[path]
	setOperation(&item, method, op)
	b.doc.Paths[path] = item
	return nil
}

func (b *Builder) MustOperation(method, path string, op Operation) *Builder {
	if err := b.Operation(method, path, op); err != nil {
		panic(err)
	}
	return b
}

func (g *GroupBuilder) Group(name string) *GroupBuilder {
	if g == nil {
		return nil
	}
	group := strings.TrimSpace(name)
	next := append([]string(nil), g.groups...)
	if group != "" {
		next = append(next, group)
	}
	return &GroupBuilder{builder: g.builder, groups: next}
}

func (g *GroupBuilder) NewOperation(method, path string) *GroupedOperationBuilder {
	return &GroupedOperationBuilder{
		group:  g,
		method: method,
		path:   path,
	}
}

func (g *GroupBuilder) Operation(method, path string, op Operation) error {
	if g == nil || g.builder == nil {
		return errors.New("openapi: group builder is nil")
	}
	op.Groups = mergeGroups(g.groups, op.Groups)
	return g.builder.Operation(method, path, op)
}

func (g *GroupBuilder) MustOperation(method, path string, op Operation) *GroupBuilder {
	if err := g.Operation(method, path, op); err != nil {
		panic(err)
	}
	return g
}

func (g *GroupedOperationBuilder) OperationID(id string) *GroupedOperationBuilder {
	g.op.OperationID(id)
	return g
}

func (g *GroupedOperationBuilder) Summary(text string) *GroupedOperationBuilder {
	g.op.Summary(text)
	return g
}

func (g *GroupedOperationBuilder) Description(text string) *GroupedOperationBuilder {
	g.op.Description(text)
	return g
}

func (g *GroupedOperationBuilder) Tag(name string) *GroupedOperationBuilder {
	g.op.Tag(name)
	return g
}

func (g *GroupedOperationBuilder) Group(name string) *GroupedOperationBuilder {
	g.op.Group(name)
	return g
}

func (g *GroupedOperationBuilder) Groups(names ...string) *GroupedOperationBuilder {
	g.op.Groups(names...)
	return g
}

func (g *GroupedOperationBuilder) Parameter(parameter Parameter) *GroupedOperationBuilder {
	g.op.Parameter(parameter)
	return g
}

func (g *GroupedOperationBuilder) ParameterRef(parameter ParameterRef) *GroupedOperationBuilder {
	g.op.ParameterRef(parameter)
	return g
}

func (g *GroupedOperationBuilder) RequestBody(body RequestBody) *GroupedOperationBuilder {
	g.op.RequestBody(body)
	return g
}

func (g *GroupedOperationBuilder) RequestBodyRef(body RequestBodyRef) *GroupedOperationBuilder {
	g.op.RequestBodyRef(body)
	return g
}

func (g *GroupedOperationBuilder) RequestJSON(schema SchemaRef, opts ...RequestBodyOption) *GroupedOperationBuilder {
	g.op.RequestJSON(schema, opts...)
	return g
}

func (g *GroupedOperationBuilder) Response(status int, response Response) *GroupedOperationBuilder {
	g.op.Response(status, response)
	return g
}

func (g *GroupedOperationBuilder) ResponseRef(status int, response ResponseRef) *GroupedOperationBuilder {
	g.op.ResponseRef(status, response)
	return g
}

func (g *GroupedOperationBuilder) ResponseJSON(status int, description string, schema SchemaRef) *GroupedOperationBuilder {
	g.op.ResponseJSON(status, description, schema)
	return g
}

func (g *GroupedOperationBuilder) Deprecated() *GroupedOperationBuilder {
	g.op.Deprecated()
	return g
}

func (g *GroupedOperationBuilder) Security(requirement SecurityRequirement) *GroupedOperationBuilder {
	g.op.Security(requirement)
	return g
}

func (g *GroupedOperationBuilder) Server(server Server) *GroupedOperationBuilder {
	g.op.Server(server)
	return g
}

func (g *GroupedOperationBuilder) Build() Operation {
	if g == nil {
		return Operation{}
	}
	op := g.op.Build()
	if g.group == nil {
		return op
	}
	op.Groups = mergeGroups(g.group.groups, op.Groups)
	return normalizeOperationGroups(op)
}

func (g *GroupedOperationBuilder) Register() error {
	if g == nil || g.group == nil || g.group.builder == nil {
		return errors.New("openapi: grouped operation builder is nil")
	}
	return g.group.builder.Operation(g.method, g.path, g.Build())
}

func (g *GroupedOperationBuilder) MustRegister() *GroupBuilder {
	if err := g.Register(); err != nil {
		panic(err)
	}
	return g.group
}

func mergeGroups(prefix, groups []string) []string {
	merged := make([]string, 0, len(prefix)+len(groups))
	merged = appendNormalizedGroups(merged, prefix)
	merged = appendNormalizedGroups(merged, groups)
	return merged
}

func normalizeOperationGroups(op Operation) Operation {
	op.Groups = appendNormalizedGroups(nil, op.Groups)
	if len(op.Tags) == 0 && len(op.Groups) > 0 {
		op.Tags = []string{op.Groups[len(op.Groups)-1]}
	}
	return op
}

func appendNormalizedGroups(dst []string, groups []string) []string {
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group != "" {
			dst = append(dst, group)
		}
	}
	return dst
}

func cloneDocument(doc *Document) (*Document, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var clone Document
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, err
	}
	return &clone, nil
}

func ensureComponents(doc *Document) {
	if doc.Components == nil {
		doc.Components = &Components{}
	}
	if doc.Components.Schemas == nil {
		doc.Components.Schemas = make(map[string]SchemaRef)
	}
	if doc.Components.Responses == nil {
		doc.Components.Responses = make(map[string]ResponseRef)
	}
	if doc.Components.Parameters == nil {
		doc.Components.Parameters = make(map[string]ParameterRef)
	}
	if doc.Components.RequestBodies == nil {
		doc.Components.RequestBodies = make(map[string]RequestBodyRef)
	}
	if doc.Components.Headers == nil {
		doc.Components.Headers = make(map[string]HeaderRef)
	}
	if doc.Components.Examples == nil {
		doc.Components.Examples = make(map[string]ExampleRef)
	}
	if doc.Components.SecuritySchemes == nil {
		doc.Components.SecuritySchemes = make(map[string]SecuritySchemeRef)
	}
	if doc.Components.Links == nil {
		doc.Components.Links = make(map[string]LinkRef)
	}
	if doc.Components.Callbacks == nil {
		doc.Components.Callbacks = make(map[string]CallbackRef)
	}
}
