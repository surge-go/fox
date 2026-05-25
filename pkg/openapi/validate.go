package openapi

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type ValidationError struct {
	Location string
	Message  string
}

type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	if len(e) == 1 {
		return e[0].Location + ": " + e[0].Message
	}
	return fmt.Sprintf("%s: %s and %d more error(s)", e[0].Location, e[0].Message, len(e)-1)
}

func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

func Validate(doc *Document) ValidationErrors {
	var errs ValidationErrors
	if doc == nil {
		return append(errs, ValidationError{Location: "document", Message: "document is nil"})
	}
	if doc.OpenAPI != defaultOpenAPIVersion {
		errs = append(errs, ValidationError{Location: "openapi", Message: "version must be 3.0.3"})
	}
	if doc.Info.Title == "" {
		errs = append(errs, ValidationError{Location: "info.title", Message: "title is required"})
	}
	if doc.Info.Version == "" {
		errs = append(errs, ValidationError{Location: "info.version", Message: "version is required"})
	}
	if len(doc.Paths) == 0 {
		errs = append(errs, ValidationError{Location: "paths", Message: "paths is required"})
	}

	operationIDs := make(map[string]string)
	for path, item := range doc.Paths {
		if !strings.HasPrefix(path, "/") {
			errs = append(errs, ValidationError{Location: "paths." + path, Message: "path must start with /"})
			continue
		}
		errs = append(errs, validatePathItem(path, item)...)
		for method, op := range operationMap(item) {
			if op.OperationID != "" {
				if prev, ok := operationIDs[op.OperationID]; ok {
					errs = append(errs, ValidationError{
						Location: "paths." + path + "." + method + ".operationId",
						Message:  fmt.Sprintf("operationId %q already exists at %s", op.OperationID, prev),
					})
				} else {
					operationIDs[op.OperationID] = "paths." + path + "." + method
				}
			}
			errs = append(errs, validateOperation(doc, path, method, op)...)
		}
	}

	errs = append(errs, validateComponentRefs(doc)...)
	return errs
}

func validatePathItem(path string, item PathItem) ValidationErrors {
	var errs ValidationErrors
	if item.Ref != "" && len(operationMap(item)) > 0 {
		errs = append(errs, ValidationError{Location: "paths." + path, Message: "$ref cannot be combined with operations"})
	}
	templateParams, templateErrs := pathTemplateParams(path)
	errs = append(errs, templateErrs...)

	for method, op := range operationMap(item) {
		declared := make(map[string]bool)
		seenParameters := make(map[string]int)
		for i, ref := range append(item.Parameters, op.Parameters...) {
			if ref.Ref != "" {
				continue
			}
			if ref.Inline == nil {
				errs = append(errs, ValidationError{Location: fmt.Sprintf("paths.%s.%s.parameters[%d]", path, method, i), Message: "parameter is empty"})
				continue
			}
			p := *ref.Inline
			location := fmt.Sprintf("paths.%s.%s.parameters[%d]", path, method, i)
			errs = append(errs, validateParameter(location, p)...)
			key := p.In + "\x00" + p.Name
			if previous, ok := seenParameters[key]; ok {
				errs = append(errs, ValidationError{Location: location, Message: fmt.Sprintf("duplicate parameter %q in %q, previously defined at parameters[%d]", p.Name, p.In, previous)})
			} else if p.In != "" && p.Name != "" {
				seenParameters[key] = i
			}
			if p.In == "path" {
				declared[p.Name] = true
				if !templateParams[p.Name] {
					errs = append(errs, ValidationError{Location: location, Message: "path parameter is not present in path template"})
				}
				if !p.Required {
					errs = append(errs, ValidationError{Location: location + ".required", Message: "path parameter must be required"})
				}
			}
		}
		for name := range templateParams {
			if !declared[name] {
				errs = append(errs, ValidationError{Location: "paths." + path + "." + method, Message: fmt.Sprintf("path template parameter %q is not declared", name)})
			}
		}
	}
	return errs
}

func validateOperation(_ *Document, path, method string, op Operation) ValidationErrors {
	var errs ValidationErrors
	if len(op.Responses) == 0 {
		errs = append(errs, ValidationError{Location: "paths." + path + "." + method + ".responses", Message: "responses is required"})
	}
	for status, response := range op.Responses {
		if !validStatusCode(status) {
			errs = append(errs, ValidationError{Location: "paths." + path + "." + method + ".responses." + status, Message: "invalid response status code"})
		}
		if response.Ref == "" && response.Inline == nil {
			errs = append(errs, ValidationError{Location: "paths." + path + "." + method + ".responses." + status, Message: "response is empty"})
		}
		if response.Inline != nil && response.Inline.Description == "" {
			errs = append(errs, ValidationError{Location: "paths." + path + "." + method + ".responses." + status + ".description", Message: "description is required"})
		}
	}
	if op.RequestBody != nil && op.RequestBody.Ref == "" && op.RequestBody.Inline == nil {
		errs = append(errs, ValidationError{Location: "paths." + path + "." + method + ".requestBody", Message: "requestBody is empty"})
	}
	if op.RequestBody != nil && op.RequestBody.Inline != nil && len(op.RequestBody.Inline.Content) == 0 {
		errs = append(errs, ValidationError{Location: "paths." + path + "." + method + ".requestBody.content", Message: "content is required"})
	}
	return errs
}

func validateParameter(location string, p Parameter) ValidationErrors {
	var errs ValidationErrors
	if p.Name == "" {
		errs = append(errs, ValidationError{Location: location + ".name", Message: "name is required"})
	}
	switch p.In {
	case "query", "header", "path", "cookie":
	default:
		errs = append(errs, ValidationError{Location: location + ".in", Message: "in must be query, header, path, or cookie"})
	}
	hasSchema := p.Schema != nil
	hasContent := len(p.Content) > 0
	if hasSchema == hasContent {
		errs = append(errs, ValidationError{Location: location, Message: "exactly one of schema or content is required"})
	}
	return errs
}

func validateHeader(location string, h Header) ValidationErrors {
	var errs ValidationErrors
	hasSchema := h.Schema != nil
	hasContent := len(h.Content) > 0
	if hasSchema == hasContent {
		errs = append(errs, ValidationError{Location: location, Message: "exactly one of schema or content is required"})
	}
	return errs
}

func pathTemplateParams(path string) (map[string]bool, ValidationErrors) {
	params := make(map[string]bool)
	var errs ValidationErrors
	for i := 0; i < len(path); i++ {
		if path[i] != '{' {
			continue
		}
		end := strings.IndexByte(path[i+1:], '}')
		if end < 0 {
			errs = append(errs, ValidationError{Location: "paths." + path, Message: "path template parameter is not closed"})
			break
		}
		name := path[i+1 : i+1+end]
		if name == "" || strings.ContainsAny(name, "{} /") {
			errs = append(errs, ValidationError{Location: "paths." + path, Message: "path template parameter is invalid"})
		} else {
			params[name] = true
		}
		i += end + 1
	}
	if strings.Contains(path, "}") {
		open := 0
		for _, ch := range path {
			if ch == '{' {
				open++
			}
			if ch == '}' {
				open--
			}
			if open < 0 {
				errs = append(errs, ValidationError{Location: "paths." + path, Message: "path template parameter is invalid"})
				break
			}
		}
	}
	return params, errs
}

var statusCodePattern = regexp.MustCompile(`^[1-5](?:[0-9]{2}|XX)$`)

func validStatusCode(code string) bool {
	return code == "default" || statusCodePattern.MatchString(code)
}

func operationMap(item PathItem) map[string]Operation {
	ops := make(map[string]Operation)
	if item.Get != nil {
		ops["get"] = *item.Get
	}
	if item.Put != nil {
		ops["put"] = *item.Put
	}
	if item.Post != nil {
		ops["post"] = *item.Post
	}
	if item.Delete != nil {
		ops["delete"] = *item.Delete
	}
	if item.Options != nil {
		ops["options"] = *item.Options
	}
	if item.Head != nil {
		ops["head"] = *item.Head
	}
	if item.Patch != nil {
		ops["patch"] = *item.Patch
	}
	if item.Trace != nil {
		ops["trace"] = *item.Trace
	}
	return ops
}

func operationsOf(item PathItem) []Operation {
	ops := operationMap(item)
	keys := make([]string, 0, len(ops))
	for key := range ops {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]Operation, 0, len(keys))
	for _, key := range keys {
		out = append(out, ops[key])
	}
	return out
}

func validateComponentRefs(doc *Document) ValidationErrors {
	var errs ValidationErrors
	for path, item := range doc.Paths {
		if item.Ref != "" {
			errs = append(errs, validateLocalRef(doc, "paths."+path+".$ref", item.Ref)...)
		}
		for i, parameter := range item.Parameters {
			errs = append(errs, validateRef(doc, fmt.Sprintf("paths.%s.parameters[%d]", path, i), parameter)...)
		}
		for method, op := range operationMap(item) {
			for i, parameter := range op.Parameters {
				location := fmt.Sprintf("paths.%s.%s.parameters[%d]", path, method, i)
				errs = append(errs, validateRef(doc, location, parameter)...)
				if parameter.Inline != nil {
					errs = append(errs, validateParameterRefs(doc, location, *parameter.Inline)...)
				}
			}
			if op.RequestBody != nil {
				errs = append(errs, validateRef(doc, "paths."+path+"."+method+".requestBody", *op.RequestBody)...)
				if op.RequestBody.Inline != nil {
					errs = append(errs, validateMediaTypes(doc, "paths."+path+"."+method+".requestBody.content", op.RequestBody.Inline.Content)...)
				}
			}
			for status, response := range op.Responses {
				errs = append(errs, validateRef(doc, "paths."+path+"."+method+".responses."+status, response)...)
				if response.Inline != nil {
					errs = append(errs, validateMediaTypes(doc, "paths."+path+"."+method+".responses."+status+".content", response.Inline.Content)...)
					for name, header := range response.Inline.Headers {
						location := "paths." + path + "." + method + ".responses." + status + ".headers." + name
						errs = append(errs, validateRef(doc, location, header)...)
						if header.Inline != nil {
							errs = append(errs, validateHeader(location, *header.Inline)...)
							errs = append(errs, validateHeaderRefs(doc, location, *header.Inline)...)
						}
					}
					for name, link := range response.Inline.Links {
						errs = append(errs, validateRef(doc, "paths."+path+"."+method+".responses."+status+".links."+name, link)...)
					}
				}
			}
		}
	}
	if doc.Components == nil {
		return errs
	}

	for name, schema := range doc.Components.Schemas {
		errs = append(errs, validateRef(doc, "components.schemas."+name, schema)...)
		errs = append(errs, validateSchemaRefRecursive(doc, "components.schemas."+name, schema, make(map[string]bool))...)
	}
	for name, response := range doc.Components.Responses {
		errs = append(errs, validateRef(doc, "components.responses."+name, response)...)
		if response.Inline != nil && response.Inline.Description == "" {
			errs = append(errs, ValidationError{Location: "components.responses." + name + ".description", Message: "description is required"})
		}
		if response.Inline != nil {
			errs = append(errs, validateMediaTypes(doc, "components.responses."+name+".content", response.Inline.Content)...)
		}
	}
	for name, parameter := range doc.Components.Parameters {
		errs = append(errs, validateRef(doc, "components.parameters."+name, parameter)...)
		if parameter.Inline != nil {
			errs = append(errs, validateParameter("components.parameters."+name, *parameter.Inline)...)
			errs = append(errs, validateParameterRefs(doc, "components.parameters."+name, *parameter.Inline)...)
		}
	}
	for name, body := range doc.Components.RequestBodies {
		errs = append(errs, validateRef(doc, "components.requestBodies."+name, body)...)
		if body.Inline != nil {
			if len(body.Inline.Content) == 0 {
				errs = append(errs, ValidationError{Location: "components.requestBodies." + name + ".content", Message: "content is required"})
			}
			errs = append(errs, validateMediaTypes(doc, "components.requestBodies."+name+".content", body.Inline.Content)...)
		}
	}
	for name, header := range doc.Components.Headers {
		errs = append(errs, validateRef(doc, "components.headers."+name, header)...)
		if header.Inline != nil {
			errs = append(errs, validateHeader("components.headers."+name, *header.Inline)...)
			errs = append(errs, validateHeaderRefs(doc, "components.headers."+name, *header.Inline)...)
		}
	}
	for name, example := range doc.Components.Examples {
		errs = append(errs, validateRef(doc, "components.examples."+name, example)...)
	}
	for name, scheme := range doc.Components.SecuritySchemes {
		errs = append(errs, validateRef(doc, "components.securitySchemes."+name, scheme)...)
		if scheme.Inline != nil {
			errs = append(errs, validateSecurityScheme("components.securitySchemes."+name, *scheme.Inline)...)
		}
	}
	for name, link := range doc.Components.Links {
		errs = append(errs, validateRef(doc, "components.links."+name, link)...)
	}
	for name, callback := range doc.Components.Callbacks {
		errs = append(errs, validateRef(doc, "components.callbacks."+name, callback)...)
	}
	return errs
}

func validateParameterRefs(doc *Document, location string, p Parameter) ValidationErrors {
	var errs ValidationErrors
	if p.Schema != nil {
		errs = append(errs, validateSchemaRefRecursive(doc, location+".schema", *p.Schema, make(map[string]bool))...)
	}
	errs = append(errs, validateMediaTypes(doc, location+".content", p.Content)...)
	return errs
}

func validateHeaderRefs(doc *Document, location string, h Header) ValidationErrors {
	var errs ValidationErrors
	if h.Schema != nil {
		errs = append(errs, validateSchemaRefRecursive(doc, location+".schema", *h.Schema, make(map[string]bool))...)
	}
	errs = append(errs, validateMediaTypes(doc, location+".content", h.Content)...)
	return errs
}

func validateMediaTypes(doc *Document, location string, content map[string]MediaType) ValidationErrors {
	var errs ValidationErrors
	for mediaType, mt := range content {
		if mt.Schema != nil {
			errs = append(errs, validateSchemaRefRecursive(doc, location+"."+mediaType+".schema", *mt.Schema, make(map[string]bool))...)
		}
		for name, example := range mt.Examples {
			errs = append(errs, validateRef(doc, location+"."+mediaType+".examples."+name, example)...)
		}
		for name, encoding := range mt.Encoding {
			for headerName, header := range encoding.Headers {
				headerLocation := location + "." + mediaType + ".encoding." + name + ".headers." + headerName
				errs = append(errs, validateRef(doc, headerLocation, header)...)
				if header.Inline != nil {
					errs = append(errs, validateHeader(headerLocation, *header.Inline)...)
					errs = append(errs, validateHeaderRefs(doc, headerLocation, *header.Inline)...)
				}
			}
		}
	}
	return errs
}

func validateSchemaRefRecursive(doc *Document, location string, ref SchemaRef, seen map[string]bool) ValidationErrors {
	var errs ValidationErrors
	errs = append(errs, validateRef(doc, location, ref)...)
	if ref.Ref != "" {
		if seen[ref.Ref] {
			return errs
		}
		seen[ref.Ref] = true
		if schema, ok := resolveSchema(doc, ref.Ref); ok && schema.Inline != nil {
			errs = append(errs, validateSchema(doc, location, *schema.Inline, seen)...)
		}
		return errs
	}
	if ref.Inline != nil {
		errs = append(errs, validateSchema(doc, location, *ref.Inline, seen)...)
	}
	return errs
}

func validateSchema(doc *Document, location string, schema Schema, seen map[string]bool) ValidationErrors {
	var errs ValidationErrors
	if schema.Type == "array" && schema.Items == nil {
		errs = append(errs, ValidationError{Location: location + ".items", Message: "items is required for array schema"})
	}
	if len(schema.Required) > 0 {
		for _, name := range schema.Required {
			if name == "" {
				errs = append(errs, ValidationError{Location: location + ".required", Message: "required property name cannot be empty"})
				continue
			}
			if _, ok := schema.Properties[name]; !ok {
				errs = append(errs, ValidationError{Location: location + ".required", Message: fmt.Sprintf("required property %q is not defined", name)})
			}
		}
	}
	for name, property := range schema.Properties {
		errs = append(errs, validateSchemaRefRecursive(doc, location+".properties."+name, property, seen)...)
	}
	if schema.Items != nil {
		errs = append(errs, validateSchemaRefRecursive(doc, location+".items", *schema.Items, seen)...)
	}
	if schema.AdditionalProperties != nil && schema.AdditionalProperties.Schema != nil {
		errs = append(errs, validateSchemaRefRecursive(doc, location+".additionalProperties", *schema.AdditionalProperties.Schema, seen)...)
	}
	if schema.AdditionalProperties != nil {
		errs = append(errs, validateAdditionalProperties(location+".additionalProperties", *schema.AdditionalProperties)...)
	}
	for i, item := range schema.AllOf {
		errs = append(errs, validateSchemaRefRecursive(doc, fmt.Sprintf("%s.allOf[%d]", location, i), item, seen)...)
	}
	for i, item := range schema.OneOf {
		errs = append(errs, validateSchemaRefRecursive(doc, fmt.Sprintf("%s.oneOf[%d]", location, i), item, seen)...)
	}
	for i, item := range schema.AnyOf {
		errs = append(errs, validateSchemaRefRecursive(doc, fmt.Sprintf("%s.anyOf[%d]", location, i), item, seen)...)
	}
	if schema.Not != nil {
		errs = append(errs, validateSchemaRefRecursive(doc, location+".not", *schema.Not, seen)...)
	}
	return errs
}

func validateAdditionalProperties(location string, properties AdditionalProperties) ValidationErrors {
	switch {
	case properties.Allowed != nil && properties.Schema == nil:
		return nil
	case properties.Allowed == nil && properties.Schema != nil:
		return nil
	case properties.Allowed == nil && properties.Schema == nil:
		return ValidationErrors{{Location: location, Message: "additionalProperties is empty"}}
	default:
		return ValidationErrors{{Location: location, Message: "additionalProperties cannot contain both boolean and schema"}}
	}
}

func validateSecurityScheme(location string, scheme SecurityScheme) ValidationErrors {
	var errs ValidationErrors
	switch scheme.Type {
	case "apiKey":
		if scheme.Name == "" {
			errs = append(errs, ValidationError{Location: location + ".name", Message: "name is required"})
		}
		switch scheme.In {
		case "query", "header", "cookie":
		default:
			errs = append(errs, ValidationError{Location: location + ".in", Message: "in must be query, header, or cookie"})
		}
	case "http":
		if scheme.Scheme == "" {
			errs = append(errs, ValidationError{Location: location + ".scheme", Message: "scheme is required"})
		}
	case "oauth2":
		errs = append(errs, validateOAuthFlows(location+".flows", scheme.Flows)...)
	case "openIdConnect":
		if scheme.OpenIDConnectURL == "" {
			errs = append(errs, ValidationError{Location: location + ".openIdConnectUrl", Message: "openIdConnectUrl is required"})
		}
	default:
		errs = append(errs, ValidationError{Location: location + ".type", Message: "type must be apiKey, http, oauth2, or openIdConnect"})
	}
	return errs
}

func validateOAuthFlows(location string, flows *OAuthFlows) ValidationErrors {
	if flows == nil {
		return ValidationErrors{{Location: location, Message: "flows is required"}}
	}
	var errs ValidationErrors
	flowCount := 0
	if flows.Implicit != nil {
		flowCount++
		errs = append(errs, validateOAuthFlow(location+".implicit", *flows.Implicit, true, false)...)
	}
	if flows.Password != nil {
		flowCount++
		errs = append(errs, validateOAuthFlow(location+".password", *flows.Password, false, true)...)
	}
	if flows.ClientCredentials != nil {
		flowCount++
		errs = append(errs, validateOAuthFlow(location+".clientCredentials", *flows.ClientCredentials, false, true)...)
	}
	if flows.AuthorizationCode != nil {
		flowCount++
		errs = append(errs, validateOAuthFlow(location+".authorizationCode", *flows.AuthorizationCode, true, true)...)
	}
	if flowCount == 0 {
		errs = append(errs, ValidationError{Location: location, Message: "at least one OAuth flow is required"})
	}
	return errs
}

func validateOAuthFlow(location string, flow OAuthFlow, requireAuthorizationURL, requireTokenURL bool) ValidationErrors {
	var errs ValidationErrors
	if requireAuthorizationURL && flow.AuthorizationURL == "" {
		errs = append(errs, ValidationError{Location: location + ".authorizationUrl", Message: "authorizationUrl is required"})
	}
	if requireTokenURL && flow.TokenURL == "" {
		errs = append(errs, ValidationError{Location: location + ".tokenUrl", Message: "tokenUrl is required"})
	}
	if flow.Scopes == nil {
		errs = append(errs, ValidationError{Location: location + ".scopes", Message: "scopes is required"})
	}
	return errs
}

type refLike interface {
	refValue() string
}

func (r SchemaRef) refValue() string         { return r.Ref }
func (r ResponseRef) refValue() string       { return r.Ref }
func (r ParameterRef) refValue() string      { return r.Ref }
func (r RequestBodyRef) refValue() string    { return r.Ref }
func (r HeaderRef) refValue() string         { return r.Ref }
func (r ExampleRef) refValue() string        { return r.Ref }
func (r LinkRef) refValue() string           { return r.Ref }
func (r CallbackRef) refValue() string       { return r.Ref }
func (r SecuritySchemeRef) refValue() string { return r.Ref }

func validateRef(doc *Document, location string, ref refLike) ValidationErrors {
	value := ref.refValue()
	if value == "" {
		return nil
	}
	return validateLocalRef(doc, location+".$ref", value)
}

func validateLocalRef(doc *Document, location, ref string) ValidationErrors {
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}
	if refExists(doc, ref) {
		return nil
	}
	return ValidationErrors{{Location: location, Message: "local reference does not exist: " + ref}}
}

func refExists(doc *Document, ref string) bool {
	if doc.Components == nil {
		return false
	}
	switch {
	case strings.HasPrefix(ref, "#/components/schemas/"):
		_, ok := doc.Components.Schemas[strings.TrimPrefix(ref, "#/components/schemas/")]
		return ok
	case strings.HasPrefix(ref, "#/components/responses/"):
		_, ok := doc.Components.Responses[strings.TrimPrefix(ref, "#/components/responses/")]
		return ok
	case strings.HasPrefix(ref, "#/components/parameters/"):
		_, ok := doc.Components.Parameters[strings.TrimPrefix(ref, "#/components/parameters/")]
		return ok
	case strings.HasPrefix(ref, "#/components/requestBodies/"):
		_, ok := doc.Components.RequestBodies[strings.TrimPrefix(ref, "#/components/requestBodies/")]
		return ok
	case strings.HasPrefix(ref, "#/components/headers/"):
		_, ok := doc.Components.Headers[strings.TrimPrefix(ref, "#/components/headers/")]
		return ok
	case strings.HasPrefix(ref, "#/components/examples/"):
		_, ok := doc.Components.Examples[strings.TrimPrefix(ref, "#/components/examples/")]
		return ok
	case strings.HasPrefix(ref, "#/components/securitySchemes/"):
		_, ok := doc.Components.SecuritySchemes[strings.TrimPrefix(ref, "#/components/securitySchemes/")]
		return ok
	case strings.HasPrefix(ref, "#/components/links/"):
		_, ok := doc.Components.Links[strings.TrimPrefix(ref, "#/components/links/")]
		return ok
	case strings.HasPrefix(ref, "#/components/callbacks/"):
		_, ok := doc.Components.Callbacks[strings.TrimPrefix(ref, "#/components/callbacks/")]
		return ok
	default:
		return false
	}
}

func resolveSchema(doc *Document, ref string) (SchemaRef, bool) {
	if doc.Components == nil || !strings.HasPrefix(ref, "#/components/schemas/") {
		return SchemaRef{}, false
	}
	schema, ok := doc.Components.Schemas[strings.TrimPrefix(ref, "#/components/schemas/")]
	return schema, ok
}
