package openapi

import (
	"bytes"
	"encoding/json"
	"errors"
)

type SchemaRef struct {
	Ref    string
	Inline *Schema
}

type ResponseRef struct {
	Ref    string
	Inline *Response
}

type ParameterRef struct {
	Ref    string
	Inline *Parameter
}

type RequestBodyRef struct {
	Ref    string
	Inline *RequestBody
}

type HeaderRef struct {
	Ref    string
	Inline *Header
}

type ExampleRef struct {
	Ref    string
	Inline *Example
}

type LinkRef struct {
	Ref    string
	Inline *Link
}

type CallbackRef struct {
	Ref    string
	Inline *Callback
}

type SecuritySchemeRef struct {
	Ref    string
	Inline *SecurityScheme
}

type AdditionalProperties struct {
	Allowed *bool
	Schema  *SchemaRef
}

func SchemaReference(ref string) SchemaRef {
	return SchemaRef{Ref: ref}
}

func SchemaInline(schema *Schema) SchemaRef {
	return SchemaRef{Inline: schema}
}

func ResponseReference(ref string) ResponseRef {
	return ResponseRef{Ref: ref}
}

func ResponseInline(response *Response) ResponseRef {
	return ResponseRef{Inline: response}
}

func ParameterReference(ref string) ParameterRef {
	return ParameterRef{Ref: ref}
}

func ParameterInline(parameter *Parameter) ParameterRef {
	return ParameterRef{Inline: parameter}
}

func RequestBodyReference(ref string) RequestBodyRef {
	return RequestBodyRef{Ref: ref}
}

func RequestBodyInline(body *RequestBody) RequestBodyRef {
	return RequestBodyRef{Inline: body}
}

func HeaderReference(ref string) HeaderRef {
	return HeaderRef{Ref: ref}
}

func HeaderInline(header *Header) HeaderRef {
	return HeaderRef{Inline: header}
}

func (r SchemaRef) MarshalJSON() ([]byte, error) { return marshalRef(r.Ref, r.Inline) }
func (r *SchemaRef) UnmarshalJSON(data []byte) error {
	return unmarshalRef(data, &r.Ref, &r.Inline)
}

func (r ResponseRef) MarshalJSON() ([]byte, error) { return marshalRef(r.Ref, r.Inline) }
func (r *ResponseRef) UnmarshalJSON(data []byte) error {
	return unmarshalRef(data, &r.Ref, &r.Inline)
}

func (r ParameterRef) MarshalJSON() ([]byte, error) { return marshalRef(r.Ref, r.Inline) }
func (r *ParameterRef) UnmarshalJSON(data []byte) error {
	return unmarshalRef(data, &r.Ref, &r.Inline)
}

func (r RequestBodyRef) MarshalJSON() ([]byte, error) { return marshalRef(r.Ref, r.Inline) }
func (r *RequestBodyRef) UnmarshalJSON(data []byte) error {
	return unmarshalRef(data, &r.Ref, &r.Inline)
}

func (r HeaderRef) MarshalJSON() ([]byte, error) { return marshalRef(r.Ref, r.Inline) }
func (r *HeaderRef) UnmarshalJSON(data []byte) error {
	return unmarshalRef(data, &r.Ref, &r.Inline)
}

func (r ExampleRef) MarshalJSON() ([]byte, error) { return marshalRef(r.Ref, r.Inline) }
func (r *ExampleRef) UnmarshalJSON(data []byte) error {
	return unmarshalRef(data, &r.Ref, &r.Inline)
}

func (r LinkRef) MarshalJSON() ([]byte, error) { return marshalRef(r.Ref, r.Inline) }
func (r *LinkRef) UnmarshalJSON(data []byte) error {
	return unmarshalRef(data, &r.Ref, &r.Inline)
}

func (r CallbackRef) MarshalJSON() ([]byte, error) { return marshalRef(r.Ref, r.Inline) }
func (r *CallbackRef) UnmarshalJSON(data []byte) error {
	return unmarshalRef(data, &r.Ref, &r.Inline)
}

func (r SecuritySchemeRef) MarshalJSON() ([]byte, error) { return marshalRef(r.Ref, r.Inline) }
func (r *SecuritySchemeRef) UnmarshalJSON(data []byte) error {
	return unmarshalRef(data, &r.Ref, &r.Inline)
}

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

func unmarshalRef[T any](data []byte, ref *string, inline **T) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("openapi: reference cannot be null")
	}

	var refObject struct {
		Ref string `json:"$ref"`
	}
	if err := json.Unmarshal(data, &refObject); err != nil {
		return err
	}
	if refObject.Ref != "" {
		*ref = refObject.Ref
		*inline = nil
		return nil
	}

	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*ref = ""
	*inline = &value
	return nil
}

func (p AdditionalProperties) MarshalJSON() ([]byte, error) {
	switch {
	case p.Allowed != nil && p.Schema == nil:
		return json.Marshal(*p.Allowed)
	case p.Allowed == nil && p.Schema != nil:
		return json.Marshal(p.Schema)
	case p.Allowed == nil && p.Schema == nil:
		return nil, errors.New("openapi: empty additionalProperties")
	default:
		return nil, errors.New("openapi: additionalProperties cannot contain both Allowed and Schema")
	}
}

func (p *AdditionalProperties) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) {
		return errors.New("openapi: additionalProperties cannot be null")
	}
	if bytes.Equal(trimmed, []byte("true")) || bytes.Equal(trimmed, []byte("false")) {
		var allowed bool
		if err := json.Unmarshal(trimmed, &allowed); err != nil {
			return err
		}
		p.Allowed = &allowed
		p.Schema = nil
		return nil
	}

	var schema SchemaRef
	if err := json.Unmarshal(trimmed, &schema); err != nil {
		return err
	}
	p.Allowed = nil
	p.Schema = &schema
	return nil
}
