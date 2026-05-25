package openapi

type Document struct {
	OpenAPI      string                `json:"openapi"`
	Info         Info                  `json:"info"`
	Servers      []Server              `json:"servers,omitempty"`
	Paths        Paths                 `json:"paths"`
	Components   *Components           `json:"components,omitempty"`
	Security     []SecurityRequirement `json:"security,omitempty"`
	Tags         []Tag                 `json:"tags,omitempty"`
	ExternalDocs *ExternalDocs         `json:"externalDocs,omitempty"`
}

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
	Groups      []string              `json:"x-groups,omitempty"`
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

type Parameter struct {
	Name        string               `json:"name"`
	In          string               `json:"in"`
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
