package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/surge-go/fox/pkg/openapi"
)

func TestSampleDocumentIsValid(t *testing.T) {
	if errs := openapi.Validate(sampleDocument()); errs.HasErrors() {
		t.Fatalf("sampleDocument() validation errors = %v", errs)
	}
}

func TestSampleDocumentUsesRequestStructTags(t *testing.T) {
	doc := sampleDocument()
	bodyRef := doc.Paths["/users"].Post.RequestBody
	if bodyRef == nil || bodyRef.Inline == nil {
		t.Fatal("create user request body missing")
	}
	schemaRef := bodyRef.Inline.Content["application/json"].Schema
	if schemaRef == nil || schemaRef.Ref == "" {
		t.Fatalf("create user schema = %#v, want component ref generated from struct", schemaRef)
	}
	schema := doc.Components.Schemas[strings.TrimPrefix(schemaRef.Ref, "#/components/schemas/")].Inline
	if schema == nil {
		t.Fatalf("schema component %q missing", schemaRef.Ref)
	}
	gender := schema.Properties["gender"].Inline
	if gender == nil {
		t.Fatal("gender schema missing")
	}
	if len(gender.Enum) != 3 || gender.Enum[0] != "male" || gender.Enum[1] != "female" || gender.Enum[2] != "unknown" {
		t.Fatalf("gender enum = %#v, want values generated from binding oneof tag", gender.Enum)
	}
	if gender.Default != "unknown" {
		t.Fatalf("gender default = %#v, want unknown", gender.Default)
	}
}

func TestCreateUserRejectsInvalidEnumValues(t *testing.T) {
	users := sampleUsers()
	body, err := json.Marshal(createUserRequest{
		Name:   "Invalid",
		Email:  "invalid@example.com",
		Gender: "other",
		Role:   "root",
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewReader(body))
	createUser(rec, req, &users)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestListUsersRejectsInvalidEnumFilters(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users?status=archived", nil)
	listUsers(rec, req, sampleUsers())

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
