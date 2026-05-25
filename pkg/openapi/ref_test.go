package openapi

import (
	"encoding/json"
	"testing"
)

func TestSchemaRefMarshalAndUnmarshal(t *testing.T) {
	ref := SchemaReference("#/components/schemas/User")
	data, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if got, want := string(data), `{"$ref":"#/components/schemas/User"}`; got != want {
		t.Fatalf("Marshal() = %s, want %s", got, want)
	}

	var decoded SchemaRef
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded.Ref != "#/components/schemas/User" || decoded.Inline != nil {
		t.Fatalf("decoded = %#v, want ref only", decoded)
	}

	data, err = json.Marshal(SchemaInline(&Schema{Type: "string"}))
	if err != nil {
		t.Fatalf("Marshal(inline) error = %v", err)
	}
	if got, want := string(data), `{"type":"string"}`; got != want {
		t.Fatalf("Marshal(inline) = %s, want %s", got, want)
	}
}

func TestAdditionalPropertiesMarshalAndUnmarshal(t *testing.T) {
	allowed := true
	data, err := json.Marshal(AdditionalProperties{Allowed: &allowed})
	if err != nil {
		t.Fatalf("Marshal(bool) error = %v", err)
	}
	if got, want := string(data), `true`; got != want {
		t.Fatalf("Marshal(bool) = %s, want %s", got, want)
	}

	var decoded AdditionalProperties
	if err := json.Unmarshal([]byte(`{"type":"integer","format":"int64"}`), &decoded); err != nil {
		t.Fatalf("Unmarshal(schema) error = %v", err)
	}
	if decoded.Schema == nil || decoded.Schema.Inline == nil {
		t.Fatalf("decoded schema = %#v, want inline schema", decoded.Schema)
	}
	if got, want := decoded.Schema.Inline.Format, "int64"; got != want {
		t.Fatalf("schema format = %q, want %q", got, want)
	}
}
