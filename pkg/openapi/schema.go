package openapi

import (
	"errors"
	"fmt"
	"hash/fnv"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type TagResolver interface {
	Resolve(field reflect.StructField, meta *FieldMeta) error
}

type TagResolverFunc func(field reflect.StructField, meta *FieldMeta) error

func (f TagResolverFunc) Resolve(field reflect.StructField, meta *FieldMeta) error {
	return f(field, meta)
}

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

type RuleTagResolverConfig struct {
	TagName string
	Strict  bool
}

func DefaultTagResolver() TagResolverFunc {
	return func(field reflect.StructField, meta *FieldMeta) error {
		if field.PkgPath != "" {
			meta.Ignore = true
			return nil
		}

		jsonName, ignored := jsonFieldName(field)
		if ignored {
			meta.Ignore = true
			return nil
		}
		if jsonName != "" && meta.Name == "" {
			meta.Name = jsonName
		}
		if meta.Name == "" {
			meta.Name = lowerFirst(field.Name)
		}

		if tag := field.Tag.Get("openapi"); tag != "" {
			if err := applyOpenAPITag(tag, meta); err != nil {
				return err
			}
		}
		if tag := field.Tag.Get("example"); tag != "" {
			value, err := parseScalarTagValue(field.Type, tag)
			if err != nil {
				return fmt.Errorf("parse example tag for %s: %w", field.Name, err)
			}
			meta.Example = value
		}
		if tag := field.Tag.Get("default"); tag != "" {
			value, err := parseScalarTagValue(field.Type, tag)
			if err != nil {
				return fmt.Errorf("parse default tag for %s: %w", field.Name, err)
			}
			meta.Default = value
		}
		return nil
	}
}

func RuleTagResolver(cfg RuleTagResolverConfig) TagResolverFunc {
	return func(field reflect.StructField, meta *FieldMeta) error {
		return applyRuleTag(field, field.Tag.Get(cfg.TagName), meta, cfg.Strict)
	}
}

func (b *Builder) SchemaOf(value any) (SchemaRef, error) {
	if value == nil {
		return SchemaRef{}, errors.New("openapi: value is nil")
	}
	return b.schemaOfType(reflect.TypeOf(value))
}

func SchemaOf[T any](b *Builder) (SchemaRef, error) {
	if b == nil {
		return SchemaRef{}, errors.New("openapi: builder is nil")
	}
	t := reflect.TypeOf((*T)(nil)).Elem()
	return b.schemaOfType(t)
}

func (b *Builder) SchemaOfType(t reflect.Type) (SchemaRef, error) {
	if b == nil {
		return SchemaRef{}, errors.New("openapi: builder is nil")
	}
	return b.schemaOfType(t)
}

func (b *Builder) schemaOfType(t reflect.Type) (SchemaRef, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	ensureComponents(&b.doc)
	g := schemaGenerator{
		builder:  b,
		building: make(map[reflect.Type]string),
	}
	return g.schemaFor(t)
}

type schemaGenerator struct {
	builder  *Builder
	building map[reflect.Type]string
}

func (g schemaGenerator) schemaFor(t reflect.Type) (SchemaRef, error) {
	if t == nil {
		return SchemaRef{}, errors.New("openapi: type is nil")
	}
	for t.Kind() == reflect.Pointer {
		ref, err := g.schemaFor(t.Elem())
		if err != nil {
			return SchemaRef{}, err
		}
		if ref.Inline != nil {
			clone := *ref.Inline
			clone.Nullable = true
			return SchemaInline(&clone), nil
		}
		return SchemaInline(&Schema{Nullable: true, AllOf: []SchemaRef{ref}}), nil
	}

	if t == reflect.TypeOf(time.Time{}) {
		return SchemaInline(&Schema{Type: "string", Format: "date-time"}), nil
	}
	if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
		return SchemaInline(&Schema{Type: "string", Format: "byte"}), nil
	}

	switch t.Kind() {
	case reflect.Bool:
		return SchemaInline(&Schema{Type: "boolean"}), nil
	case reflect.String:
		return SchemaInline(&Schema{Type: "string"}), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		return SchemaInline(&Schema{Type: "integer", Format: "int32"}), nil
	case reflect.Int64:
		return SchemaInline(&Schema{Type: "integer", Format: "int64"}), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return SchemaInline(&Schema{Type: "integer", Format: "int32", Minimum: floatPtr(0)}), nil
	case reflect.Uint64:
		return SchemaInline(&Schema{Type: "integer", Format: "int64", Minimum: floatPtr(0)}), nil
	case reflect.Float32:
		return SchemaInline(&Schema{Type: "number", Format: "float"}), nil
	case reflect.Float64:
		return SchemaInline(&Schema{Type: "number", Format: "double"}), nil
	case reflect.Slice, reflect.Array:
		item, err := g.schemaFor(t.Elem())
		if err != nil {
			return SchemaRef{}, err
		}
		return SchemaInline(&Schema{Type: "array", Items: &item}), nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return SchemaRef{}, fmt.Errorf("openapi: map key type %s is not supported", t.Key())
		}
		value, err := g.schemaFor(t.Elem())
		if err != nil {
			return SchemaRef{}, err
		}
		return SchemaInline(&Schema{
			Type: "object",
			AdditionalProperties: &AdditionalProperties{
				Schema: &value,
			},
		}), nil
	case reflect.Struct:
		return g.structSchema(t)
	case reflect.Interface:
		return SchemaInline(&Schema{}), nil
	default:
		return SchemaRef{}, fmt.Errorf("openapi: type %s is not supported", t)
	}
}

func (g schemaGenerator) structSchema(t reflect.Type) (SchemaRef, error) {
	if t.Name() != "" {
		name := g.componentNameForType(t)
		ref := SchemaReference("#/components/schemas/" + name)
		if _, ok := g.builder.doc.Components.Schemas[name]; ok {
			return ref, nil
		}
		if _, ok := g.building[t]; ok {
			return ref, nil
		}

		schema := &Schema{Type: "object", Properties: make(map[string]SchemaRef)}
		g.builder.doc.Components.Schemas[name] = SchemaInline(schema)
		g.building[t] = name
		if err := g.fillStructSchema(t, schema); err != nil {
			delete(g.builder.doc.Components.Schemas, name)
			delete(g.building, t)
			return SchemaRef{}, err
		}
		delete(g.building, t)
		return ref, nil
	}

	schema := &Schema{Type: "object", Properties: make(map[string]SchemaRef)}
	if err := g.fillStructSchema(t, schema); err != nil {
		return SchemaRef{}, err
	}
	return SchemaInline(schema), nil
}

func (g schemaGenerator) componentNameForType(t reflect.Type) string {
	if name, ok := g.builder.schemaTypes[t]; ok {
		return name
	}

	name := componentName(t)
	if owner, ok := g.builder.schemaNames[name]; ok && owner != t {
		name = componentNameWithHash(t)
	} else if _, exists := g.builder.doc.Components.Schemas[name]; exists {
		name = componentNameWithHash(t)
	}

	base := name
	for i := 2; ; i++ {
		owner, owned := g.builder.schemaNames[name]
		_, exists := g.builder.doc.Components.Schemas[name]
		if (!owned || owner == t) && !exists {
			break
		}
		name = fmt.Sprintf("%s_%d", base, i)
	}

	g.builder.schemaTypes[t] = name
	g.builder.schemaNames[name] = t
	return name
}

func (g schemaGenerator) fillStructSchema(t reflect.Type, schema *Schema) error {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		meta := FieldMeta{}
		for _, resolver := range g.builder.resolvers {
			if err := resolver.Resolve(field, &meta); err != nil {
				return err
			}
			if meta.Ignore {
				break
			}
		}
		if meta.Ignore {
			continue
		}
		if meta.Name == "" {
			meta.Name = lowerFirst(field.Name)
		}

		fieldSchema, err := g.schemaFor(field.Type)
		if err != nil {
			return fmt.Errorf("schema field %s: %w", field.Name, err)
		}
		fieldSchema = applyFieldMeta(fieldSchema, meta)
		schema.Properties[meta.Name] = fieldSchema
		if meta.Required != nil && *meta.Required {
			schema.Required = append(schema.Required, meta.Name)
		}
	}
	if len(schema.Properties) == 0 {
		schema.Properties = nil
	}
	return nil
}

func applyFieldMeta(ref SchemaRef, meta FieldMeta) SchemaRef {
	var schema *Schema
	if ref.Inline != nil {
		clone := *ref.Inline
		schema = &clone
	} else {
		schema = &Schema{AllOf: []SchemaRef{ref}}
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
	applyConstraints(schema, meta.Constraints)
	return SchemaInline(schema)
}

func applyConstraints(schema *Schema, c FieldConstraints) {
	if c.Minimum != nil {
		schema.Minimum = c.Minimum
	}
	if c.Maximum != nil {
		schema.Maximum = c.Maximum
	}
	if c.MinLength != nil {
		schema.MinLength = c.MinLength
	}
	if c.MaxLength != nil {
		schema.MaxLength = c.MaxLength
	}
	if c.MinItems != nil {
		schema.MinItems = c.MinItems
	}
	if c.MaxItems != nil {
		schema.MaxItems = c.MaxItems
	}
	if c.Pattern != "" {
		schema.Pattern = c.Pattern
	}
	if len(c.Enum) > 0 {
		schema.Enum = c.Enum
	}
	if c.Format != "" {
		schema.Format = c.Format
	}
}

func applyOpenAPITag(tag string, meta *FieldMeta) error {
	for _, part := range strings.Split(tag, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, hasValue := strings.Cut(part, "=")
		switch key {
		case "-":
			meta.Ignore = true
		case "required":
			v := true
			meta.Required = &v
		case "nullable":
			v := true
			meta.Nullable = &v
		case "deprecated":
			meta.Deprecated = true
		case "name":
			if !hasValue || value == "" {
				return errors.New("openapi: name tag requires a value")
			}
			meta.Name = value
		case "description", "desc":
			meta.Description = value
		default:
			return fmt.Errorf("openapi: unknown openapi tag option %q", key)
		}
	}
	return nil
}

func applyRuleTag(field reflect.StructField, value string, meta *FieldMeta, strict bool) error {
	if value == "" {
		return nil
	}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, raw, hasValue := strings.Cut(part, "=")
		switch key {
		case "required":
			v := true
			meta.Required = &v
		case "desc", "description":
			if hasValue {
				meta.Description = raw
			}
		case "min", "max", "len":
			if !hasValue {
				if strict {
					return fmt.Errorf("openapi: rule %s requires a value", key)
				}
				continue
			}
			if err := applyRangeRule(field.Type, key, raw, &meta.Constraints); err != nil && strict {
				return err
			}
		case "oneof":
			if hasValue {
				for _, item := range strings.Fields(raw) {
					value, err := parseScalarTagValue(field.Type, item)
					if err != nil {
						if strict {
							return fmt.Errorf("openapi: parse oneof value %q for %s: %w", item, field.Name, err)
						}
						continue
					}
					meta.Constraints.Enum = append(meta.Constraints.Enum, value)
				}
			}
		case "email":
			meta.Constraints.Format = "email"
		case "url", "uri":
			meta.Constraints.Format = "uri"
		case "uuid":
			meta.Constraints.Format = "uuid"
		case "datetime":
			meta.Constraints.Format = "date-time"
		default:
			if strict {
				return fmt.Errorf("openapi: unknown rule %q", key)
			}
		}
	}
	return nil
}

func applyRangeRule(t reflect.Type, key, raw string, c *FieldConstraints) error {
	base := derefType(t)
	switch base.Kind() {
	case reflect.String:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return err
		}
		switch key {
		case "min":
			c.MinLength = &n
		case "max":
			c.MaxLength = &n
		case "len":
			c.MinLength = &n
			c.MaxLength = &n
		}
	case reflect.Slice, reflect.Array:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return err
		}
		switch key {
		case "min":
			c.MinItems = &n
		case "max":
			c.MaxItems = &n
		case "len":
			c.MinItems = &n
			c.MaxItems = &n
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return err
		}
		switch key {
		case "min":
			c.Minimum = &n
		case "max":
			c.Maximum = &n
		case "len":
			c.Minimum = &n
			c.Maximum = &n
		}
	default:
		return fmt.Errorf("openapi: rule %s is not supported for %s", key, t)
	}
	return nil
}

func parseScalarTagValue(t reflect.Type, raw string) (any, error) {
	base := derefType(t)
	switch base.Kind() {
	case reflect.String:
		return raw, nil
	case reflect.Bool:
		return strconv.ParseBool(raw)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.ParseInt(raw, 10, 64)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.ParseUint(raw, 10, 64)
	case reflect.Float32, reflect.Float64:
		return strconv.ParseFloat(raw, 64)
	default:
		return raw, nil
	}
}

func jsonFieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", true
	}
	if tag == "" {
		return "", false
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "-" {
		return "", true
	}
	return name, false
}

func componentName(t reflect.Type) string {
	if t.PkgPath() == "" {
		return t.Name()
	}
	parts := strings.Split(t.PkgPath(), "/")
	pkg := parts[len(parts)-1]
	return pkg + "_" + t.Name()
}

func componentNameWithHash(t reflect.Type) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(t.PkgPath()))
	_, _ = h.Write([]byte("/"))
	_, _ = h.Write([]byte(t.Name()))
	return fmt.Sprintf("%s_%06x", componentName(t), h.Sum32()&0xffffff)
}

func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
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

func floatPtr(v float64) *float64 {
	return &v
}
