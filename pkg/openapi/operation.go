package openapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type OperationBuilder struct {
	op Operation
}

type ParameterOption func(*Parameter)
type RequestBodyOption func(*RequestBody)
type ResponseOption func(*Response)

func NewOperation() *OperationBuilder {
	return &OperationBuilder{}
}

func (b *OperationBuilder) OperationID(id string) *OperationBuilder {
	b.op.OperationID = id
	return b
}

func (b *OperationBuilder) Summary(text string) *OperationBuilder {
	b.op.Summary = text
	return b
}

func (b *OperationBuilder) Description(text string) *OperationBuilder {
	b.op.Description = text
	return b
}

func (b *OperationBuilder) Tag(name string) *OperationBuilder {
	b.op.Tags = append(b.op.Tags, name)
	return b
}

func (b *OperationBuilder) Group(name string) *OperationBuilder {
	name = strings.TrimSpace(name)
	if name != "" {
		b.op.Groups = append(b.op.Groups, name)
	}
	return b
}

func (b *OperationBuilder) Groups(names ...string) *OperationBuilder {
	for _, name := range names {
		b.Group(name)
	}
	return b
}

func (b *OperationBuilder) Parameter(parameter Parameter) *OperationBuilder {
	b.op.Parameters = append(b.op.Parameters, ParameterRef{Inline: &parameter})
	return b
}

func (b *OperationBuilder) ParameterRef(parameter ParameterRef) *OperationBuilder {
	b.op.Parameters = append(b.op.Parameters, parameter)
	return b
}

func (b *OperationBuilder) RequestBody(body RequestBody) *OperationBuilder {
	ref := RequestBodyRef{Inline: &body}
	b.op.RequestBody = &ref
	return b
}

func (b *OperationBuilder) RequestBodyRef(body RequestBodyRef) *OperationBuilder {
	b.op.RequestBody = &body
	return b
}

func (b *OperationBuilder) RequestJSON(schema SchemaRef, opts ...RequestBodyOption) *OperationBuilder {
	return b.RequestBody(JSONBody(schema, opts...))
}

func (b *OperationBuilder) Response(status int, response Response) *OperationBuilder {
	return b.ResponseRef(status, ResponseRef{Inline: &response})
}

func (b *OperationBuilder) ResponseRef(status int, response ResponseRef) *OperationBuilder {
	if b.op.Responses == nil {
		b.op.Responses = make(Responses)
	}
	b.op.Responses[strconv.Itoa(status)] = response
	return b
}

func (b *OperationBuilder) ResponseJSON(status int, description string, schema SchemaRef) *OperationBuilder {
	return b.Response(status, JSONResponse(status, description, schema))
}

func (b *OperationBuilder) Deprecated() *OperationBuilder {
	b.op.Deprecated = true
	return b
}

func (b *OperationBuilder) Security(requirement SecurityRequirement) *OperationBuilder {
	b.op.Security = append(b.op.Security, requirement)
	return b
}

func (b *OperationBuilder) Server(server Server) *OperationBuilder {
	b.op.Servers = append(b.op.Servers, server)
	return b
}

func (b *OperationBuilder) Build() Operation {
	return normalizeOperationGroups(b.op)
}

func Query(name string, schema SchemaRef, opts ...ParameterOption) Parameter {
	return parameter("query", name, schema, opts...)
}

func Path(name string, schema SchemaRef, opts ...ParameterOption) Parameter {
	p := parameter("path", name, schema, opts...)
	p.Required = true
	return p
}

func HeaderParam(name string, schema SchemaRef, opts ...ParameterOption) Parameter {
	return parameter("header", name, schema, opts...)
}

func Cookie(name string, schema SchemaRef, opts ...ParameterOption) Parameter {
	return parameter("cookie", name, schema, opts...)
}

func WithParameterDescription(description string) ParameterOption {
	return func(p *Parameter) {
		p.Description = description
	}
}

func WithParameterRequired(required bool) ParameterOption {
	return func(p *Parameter) {
		p.Required = required
	}
}

func JSONBody(schema SchemaRef, opts ...RequestBodyOption) RequestBody {
	return requestBody("application/json", schema, opts...)
}

func FormBody(schema SchemaRef, opts ...RequestBodyOption) RequestBody {
	return requestBody("application/x-www-form-urlencoded", schema, opts...)
}

func MultipartBody(schema SchemaRef, opts ...RequestBodyOption) RequestBody {
	return requestBody("multipart/form-data", schema, opts...)
}

func WithRequestBodyDescription(description string) RequestBodyOption {
	return func(b *RequestBody) {
		b.Description = description
	}
}

func WithRequestBodyRequired(required bool) RequestBodyOption {
	return func(b *RequestBody) {
		b.Required = required
	}
}

func JSONResponse(_ int, description string, schema SchemaRef, opts ...ResponseOption) Response {
	schemaRef := schema
	resp := Response{
		Description: description,
		Content: map[string]MediaType{
			"application/json": {Schema: &schemaRef},
		},
	}
	for _, opt := range opts {
		opt(&resp)
	}
	return resp
}

func EmptyResponse(_ int, description string, opts ...ResponseOption) Response {
	resp := Response{Description: description}
	for _, opt := range opts {
		opt(&resp)
	}
	return resp
}

func parameter(in, name string, schema SchemaRef, opts ...ParameterOption) Parameter {
	schemaRef := schema
	p := Parameter{
		Name:   name,
		In:     in,
		Schema: &schemaRef,
	}
	for _, opt := range opts {
		opt(&p)
	}
	return p
}

func requestBody(contentType string, schema SchemaRef, opts ...RequestBodyOption) RequestBody {
	schemaRef := schema
	body := RequestBody{
		Content: map[string]MediaType{
			contentType: {Schema: &schemaRef},
		},
	}
	for _, opt := range opts {
		opt(&body)
	}
	return body
}

func validateOperationRegistration(doc *Document, method, path string, op Operation) error {
	method = strings.ToUpper(method)
	if !isHTTPMethod(method) {
		return fmt.Errorf("openapi: unsupported method %q", method)
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("openapi: path %q must start with /", path)
	}
	if op.OperationID != "" {
		for existingPath, item := range doc.Paths {
			for _, existing := range operationsOf(item) {
				if existing.OperationID == op.OperationID {
					return fmt.Errorf("openapi: duplicate operationId %q at %s", op.OperationID, existingPath)
				}
			}
		}
	}
	item := PathItem{}
	setOperation(&item, method, op)
	if errs := validatePathItem(path, item); errs.HasErrors() {
		return errs
	}
	return nil
}

func setOperation(item *PathItem, method string, op Operation) {
	switch strings.ToUpper(method) {
	case http.MethodGet:
		item.Get = &op
	case http.MethodPut:
		item.Put = &op
	case http.MethodPost:
		item.Post = &op
	case http.MethodDelete:
		item.Delete = &op
	case http.MethodOptions:
		item.Options = &op
	case http.MethodHead:
		item.Head = &op
	case http.MethodPatch:
		item.Patch = &op
	case http.MethodTrace:
		item.Trace = &op
	}
}

func isHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPut, http.MethodPost, http.MethodDelete,
		http.MethodOptions, http.MethodHead, http.MethodPatch, http.MethodTrace:
		return true
	default:
		return false
	}
}
