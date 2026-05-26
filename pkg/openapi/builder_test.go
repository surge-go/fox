package openapi

import (
	"reflect"
	"strings"
	"testing"

	modela "github.com/surge-go/fox/pkg/openapi/internal/schemafixtures/a/model"
	modelb "github.com/surge-go/fox/pkg/openapi/internal/schemafixtures/b/model"
)

func TestBuilderOperationValidateAndJSON(t *testing.T) {
	doc := New(Config{
		Info: Info{Title: "User API", Version: "1.0.0"},
	})

	userSchema, err := doc.AddSchema("User", &Schema{
		Type: "object",
		Properties: map[string]SchemaRef{
			"id": SchemaInline(&Schema{Type: "integer", Format: "int64"}),
		},
	})
	if err != nil {
		t.Fatalf("AddSchema() error = %v", err)
	}

	op := NewOperation().
		OperationID("getUser").
		Parameter(Path("id", SchemaInline(&Schema{Type: "integer", Format: "int64"}))).
		ResponseJSON(200, "OK", userSchema).
		Build()

	if err := doc.Operation("GET", "/users/{id}", op); err != nil {
		t.Fatalf("Operation() error = %v", err)
	}
	if errs := doc.Validate(); errs.HasErrors() {
		t.Fatalf("Validate() errors = %v", errs)
	}

	payload, err := doc.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if !strings.Contains(string(payload), `"$ref":"#/components/schemas/User"`) {
		t.Fatalf("JSON() = %s, want schema ref", payload)
	}
}

func TestBuilderGroupOperationAddsXGroupsAndTag(t *testing.T) {
	doc := New(Config{
		Info: Info{Title: "API", Version: "1.0.0"},
	})

	op := NewOperation().
		OperationID("login").
		Group("认证").
		ResponseJSON(200, "OK", SchemaInline(&Schema{Type: "object"})).
		Build()

	doc.Group("管理端").Group("用户").MustOperation("POST", "/login", op)

	got := doc.Document().Paths["/login"].Post
	if got == nil {
		t.Fatal("operation missing")
	}
	if want := "管理端,用户,认证"; strings.Join(got.Groups, ",") != want {
		t.Fatalf("groups = %#v, want %s", got.Groups, want)
	}
	if want := "认证"; strings.Join(got.Tags, ",") != want {
		t.Fatalf("tags = %#v, want %s", got.Tags, want)
	}

	payload, err := doc.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if !strings.Contains(string(payload), `"x-groups":["管理端","用户","认证"]`) {
		t.Fatalf("JSON() = %s, want x-groups", payload)
	}
}

func TestGroupBuilderNewOperationRegistersWithGroupContext(t *testing.T) {
	doc := New(Config{
		Info: Info{Title: "API", Version: "1.0.0"},
	})

	doc.Group("业务接口").
		Group("用户").
		NewOperation("POST", "/users").
		OperationID("createUser").
		Summary("创建用户").
		RequestJSON(SchemaInline(&Schema{Type: "object"}), WithRequestBodyRequired(true)).
		ResponseJSON(201, "Created", SchemaInline(&Schema{Type: "object"})).
		MustRegister()

	got := doc.Document().Paths["/users"].Post
	if got == nil {
		t.Fatal("operation missing")
	}
	if want := "业务接口,用户"; strings.Join(got.Groups, ",") != want {
		t.Fatalf("groups = %#v, want %s", got.Groups, want)
	}
	if want := "用户"; strings.Join(got.Tags, ",") != want {
		t.Fatalf("tags = %#v, want %s", got.Tags, want)
	}
	if got.OperationID != "createUser" {
		t.Fatalf("operationId = %q, want createUser", got.OperationID)
	}
}

func TestGroupBuilderNewOperationBuildIncludesGroupContext(t *testing.T) {
	doc := New(Config{
		Info: Info{Title: "API", Version: "1.0.0"},
	})

	op := doc.Group("业务接口").
		Group("用户").
		NewOperation("POST", "/users").
		Group("创建").
		ResponseJSON(201, "Created", SchemaInline(&Schema{Type: "object"})).
		Build()

	if want := "业务接口,用户,创建"; strings.Join(op.Groups, ",") != want {
		t.Fatalf("groups = %#v, want %s", op.Groups, want)
	}
	if want := "创建"; strings.Join(op.Tags, ",") != want {
		t.Fatalf("tags = %#v, want %s", op.Tags, want)
	}
}

func TestValidateReportsInvalidReference(t *testing.T) {
	op := NewOperation().
		ResponseRef(200, ResponseRef{Ref: "#/components/responses/Missing"}).
		Build()
	doc := &Document{
		OpenAPI: "3.0.3",
		Info:    Info{Title: "API", Version: "1.0.0"},
		Paths: Paths{
			"/users": {
				Get: &op,
			},
		},
		Components: &Components{},
	}

	errs := Validate(doc)
	if !errs.HasErrors() {
		t.Fatal("Validate() returned no errors, want missing ref error")
	}
	if !strings.Contains(errs.Error(), "local reference does not exist") {
		t.Fatalf("Validate() = %v, want missing ref error", errs)
	}
}

func TestValidateReportsInvalidReferenceWhenComponentsMissing(t *testing.T) {
	op := NewOperation().
		ResponseRef(200, ResponseRef{Ref: "#/components/responses/Missing"}).
		Build()
	doc := &Document{
		OpenAPI: "3.0.3",
		Info:    Info{Title: "API", Version: "1.0.0"},
		Paths: Paths{
			"/users": {
				Get: &op,
			},
		},
	}

	errs := Validate(doc)
	if !errs.HasErrors() {
		t.Fatal("Validate() returned no errors, want missing ref error")
	}
	if !strings.Contains(errs.Error(), "#/components/responses/Missing") {
		t.Fatalf("Validate() = %v, want missing response ref error", errs)
	}
}

func TestValidateReportsNestedSchemaReference(t *testing.T) {
	doc := &Document{
		OpenAPI: "3.0.3",
		Info:    Info{Title: "API", Version: "1.0.0"},
		Paths: Paths{
			"/users": {
				Get: &Operation{
					Responses: Responses{
						"200": ResponseRef{Inline: &Response{
							Description: "OK",
							Content: map[string]MediaType{
								"application/json": {
									Schema: &SchemaRef{Ref: "#/components/schemas/Missing"},
								},
							},
						}},
					},
				},
			},
		},
		Components: &Components{Schemas: map[string]SchemaRef{}},
	}

	errs := Validate(doc)
	if !errs.HasErrors() {
		t.Fatal("Validate() returned no errors, want nested missing ref error")
	}
	if !strings.Contains(errs.Error(), "#/components/schemas/Missing") {
		t.Fatalf("Validate() = %v, want nested schema ref error", errs)
	}
}

type schemaUser struct {
	Name   string `json:"name" binding:"required,desc=用户名,min=2,max=32"`
	Email  string `json:"email" validate:"required,email"`
	Gender string `json:"gender" binding:"oneof=male female unknown"`
	Age    uint   `json:"age" binding:"min=1,max=120"`
}

type taggedProfile struct {
	Code      string   `json:"code" openapi:"name=code_value,desc=编码,nullable,deprecated" example:"A001" default:"A000"`
	Nickname  string   `json:"nickname" binding:"len=8"`
	Homepage  string   `json:"homepage" validate:"url"`
	UUID      string   `json:"uuid" validate:"uuid"`
	CreatedAt string   `json:"createdAt" validate:"datetime"`
	Tags      []string `json:"tags" binding:"min=1,max=5"`
	Score     int      `json:"score" binding:"min=0,max=100"`
	Ignored   string   `json:"ignored" openapi:"-"`
}

type numericChoice struct {
	Level int `json:"level" binding:"oneof=1 2 3"`
}

func TestSchemaOfWithRuleResolvers(t *testing.T) {
	doc := New(Config{
		Info: Info{Title: "API", Version: "1.0.0"},
		TagResolvers: []TagResolver{
			DefaultTagResolver(),
			RuleTagResolver(RuleTagResolverConfig{TagName: "binding", Strict: true}),
			RuleTagResolver(RuleTagResolverConfig{TagName: "validate", Strict: true}),
		},
	})

	ref, err := SchemaOf[schemaUser](doc)
	if err != nil {
		t.Fatalf("SchemaOf() error = %v", err)
	}
	if ref.Ref == "" {
		t.Fatalf("SchemaOf() = %#v, want component ref", ref)
	}

	schema := doc.Document().Components.Schemas["openapi_schemaUser"].Inline
	if schema == nil {
		t.Fatal("schema component missing")
	}
	if got, want := schema.Properties["name"].Inline.Description, "用户名"; got != want {
		t.Fatalf("name description = %q, want %q", got, want)
	}
	if got, want := schema.Properties["email"].Inline.Format, "email"; got != want {
		t.Fatalf("email format = %q, want %q", got, want)
	}
	genderEnum := schema.Properties["gender"].Inline.Enum
	if len(genderEnum) != 3 || genderEnum[0] != "male" || genderEnum[1] != "female" || genderEnum[2] != "unknown" {
		t.Fatalf("gender enum = %#v, want male/female/unknown", genderEnum)
	}
	if got, want := schema.Required, []string{"name", "email"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("required = %#v, want %#v", got, want)
	}
}

func TestSchemaOfConvertsNumericOneOfValues(t *testing.T) {
	doc := New(Config{
		Info: Info{Title: "API", Version: "1.0.0"},
		TagResolvers: []TagResolver{
			DefaultTagResolver(),
			RuleTagResolver(RuleTagResolverConfig{TagName: "binding", Strict: true}),
		},
	})

	if _, err := SchemaOf[numericChoice](doc); err != nil {
		t.Fatalf("SchemaOf() error = %v", err)
	}
	schema := doc.Document().Components.Schemas["openapi_numericChoice"].Inline
	if schema == nil {
		t.Fatal("schema component missing")
	}
	enum := schema.Properties["level"].Inline.Enum
	if len(enum) != 3 || enum[0] != float64(1) || enum[1] != float64(2) || enum[2] != float64(3) {
		t.Fatalf("level enum = %#v, want numeric values 1/2/3", enum)
	}
}

func TestSchemaOfTypeGeneratesSchemaFromReflectType(t *testing.T) {
	doc := New(Config{
		Info: Info{Title: "API", Version: "1.0.0"},
		TagResolvers: []TagResolver{
			DefaultTagResolver(),
			RuleTagResolver(RuleTagResolverConfig{TagName: "binding", Strict: true}),
		},
	})

	ref, err := doc.SchemaOfType(reflect.TypeOf(schemaUser{}))
	if err != nil {
		t.Fatalf("SchemaOfType() error = %v", err)
	}
	if ref.Ref == "" {
		t.Fatalf("SchemaOfType() = %#v, want component ref", ref)
	}

	schema := doc.Document().Components.Schemas["openapi_schemaUser"].Inline
	if schema == nil {
		t.Fatal("schema component missing")
	}
	if got, want := schema.Properties["name"].Inline.Description, "用户名"; got != want {
		t.Fatalf("name description = %q, want %q", got, want)
	}
}

func TestSchemaOfTypeRejectsNilBuilder(t *testing.T) {
	var doc *Builder
	if _, err := doc.SchemaOfType(reflect.TypeOf(schemaUser{})); err == nil {
		t.Fatal("SchemaOfType() error = nil, want error")
	}
}

func TestSchemaOfWithAdditionalTags(t *testing.T) {
	doc := New(Config{
		Info: Info{Title: "API", Version: "1.0.0"},
		TagResolvers: []TagResolver{
			DefaultTagResolver(),
			RuleTagResolver(RuleTagResolverConfig{TagName: "binding", Strict: true}),
			RuleTagResolver(RuleTagResolverConfig{TagName: "validate", Strict: true}),
		},
	})

	ref, err := SchemaOf[taggedProfile](doc)
	if err != nil {
		t.Fatalf("SchemaOf() error = %v", err)
	}
	if ref.Ref == "" {
		t.Fatalf("SchemaOf() = %#v, want component ref", ref)
	}

	schema := doc.Document().Components.Schemas["openapi_taggedProfile"].Inline
	if schema == nil {
		t.Fatal("schema component missing")
	}
	if _, ok := schema.Properties["ignored"]; ok {
		t.Fatal("ignored field should not be present")
	}

	code := schema.Properties["code_value"].Inline
	if code == nil {
		t.Fatal("code_value schema missing")
	}
	if code.Description != "编码" || !code.Nullable || !code.Deprecated || code.Example != "A001" || code.Default != "A000" {
		t.Fatalf("code schema = %#v, want description/nullable/deprecated/example/default", code)
	}

	nickname := schema.Properties["nickname"].Inline
	if nickname.MinLength == nil || *nickname.MinLength != 8 || nickname.MaxLength == nil || *nickname.MaxLength != 8 {
		t.Fatalf("nickname length = min %#v max %#v, want 8/8", nickname.MinLength, nickname.MaxLength)
	}
	tags := schema.Properties["tags"].Inline
	if tags.MinItems == nil || *tags.MinItems != 1 || tags.MaxItems == nil || *tags.MaxItems != 5 {
		t.Fatalf("tags items = min %#v max %#v, want 1/5", tags.MinItems, tags.MaxItems)
	}
	score := schema.Properties["score"].Inline
	if score.Minimum == nil || *score.Minimum != 0 || score.Maximum == nil || *score.Maximum != 100 {
		t.Fatalf("score range = min %#v max %#v, want 0/100", score.Minimum, score.Maximum)
	}
	if got, want := schema.Properties["homepage"].Inline.Format, "uri"; got != want {
		t.Fatalf("homepage format = %q, want %q", got, want)
	}
	if got, want := schema.Properties["uuid"].Inline.Format, "uuid"; got != want {
		t.Fatalf("uuid format = %q, want %q", got, want)
	}
	if got, want := schema.Properties["createdAt"].Inline.Format, "date-time"; got != want {
		t.Fatalf("createdAt format = %q, want %q", got, want)
	}
}

func TestValidateReportsDuplicateParameters(t *testing.T) {
	doc := &Document{
		OpenAPI: "3.0.3",
		Info:    Info{Title: "API", Version: "1.0.0"},
		Paths: Paths{
			"/users": {
				Get: &Operation{
					Parameters: []ParameterRef{
						ParameterInline(&Parameter{Name: "status", In: "query", Schema: schemaPtrForTest(SchemaInline(&Schema{Type: "string"}))}),
						ParameterInline(&Parameter{Name: "status", In: "query", Schema: schemaPtrForTest(SchemaInline(&Schema{Type: "string"}))}),
					},
					Responses: Responses{
						"200": ResponseInline(&Response{Description: "OK"}),
					},
				},
			},
		},
	}

	errs := Validate(doc)
	if !validationErrorsContain(errs, "duplicate parameter") {
		t.Fatalf("Validate() = %v, want duplicate parameter error", errs)
	}
}

func TestSchemaOfAvoidsComponentNameCollision(t *testing.T) {
	doc := New(Config{
		Info: Info{Title: "API", Version: "1.0.0"},
	})

	first, err := SchemaOf[modela.User](doc)
	if err != nil {
		t.Fatalf("SchemaOf[modela.User]() error = %v", err)
	}
	second, err := SchemaOf[modelb.User](doc)
	if err != nil {
		t.Fatalf("SchemaOf[modelb.User]() error = %v", err)
	}
	if first.Ref == "" || second.Ref == "" {
		t.Fatalf("refs = %#v %#v, want component refs", first, second)
	}
	if first.Ref == second.Ref {
		t.Fatalf("refs both use %q, want distinct component refs", first.Ref)
	}

	schemas := doc.Document().Components.Schemas
	if len(schemas) != 2 {
		t.Fatalf("schema count = %d, want 2: %#v", len(schemas), schemas)
	}
	if schemas[strings.TrimPrefix(first.Ref, "#/components/schemas/")].Inline.Properties["id"].Inline == nil {
		t.Fatalf("first schema missing id property")
	}
	if schemas[strings.TrimPrefix(second.Ref, "#/components/schemas/")].Inline.Properties["name"].Inline == nil {
		t.Fatalf("second schema missing name property")
	}
}

func TestValidateReportsSchemaStructureErrors(t *testing.T) {
	doc := &Document{
		OpenAPI: "3.0.3",
		Info:    Info{Title: "API", Version: "1.0.0"},
		Paths: Paths{
			"/users": {
				Post: &Operation{
					RequestBody: &RequestBodyRef{Inline: &RequestBody{}},
					Responses: Responses{
						"200": ResponseRef{Inline: &Response{Description: "OK"}},
					},
				},
			},
		},
		Components: &Components{
			Schemas: map[string]SchemaRef{
				"Broken": SchemaInline(&Schema{
					Type:     "array",
					Required: []string{"missing"},
				}),
			},
			RequestBodies: map[string]RequestBodyRef{
				"Empty": {Inline: &RequestBody{}},
			},
			SecuritySchemes: map[string]SecuritySchemeRef{
				"APIKey": {Inline: &SecurityScheme{Type: "apiKey"}},
			},
		},
	}

	errs := Validate(doc)
	if !errs.HasErrors() {
		t.Fatal("Validate() returned no errors, want schema structure errors")
	}
	for _, want := range []string{"requestBody.content", "components.requestBodies.Empty.content", "components.schemas.Broken.items", "required property", "components.securitySchemes.APIKey.name"} {
		if !validationErrorsContain(errs, want) {
			t.Fatalf("Validate() = %v, want %q", errs, want)
		}
	}
}

func validationErrorsContain(errs ValidationErrors, want string) bool {
	for _, err := range errs {
		if strings.Contains(err.Location, want) || strings.Contains(err.Message, want) {
			return true
		}
	}
	return false
}

func schemaPtrForTest(schema SchemaRef) *SchemaRef {
	return &schema
}
