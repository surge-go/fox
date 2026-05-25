package ui

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/surge-go/fox/pkg/openapi"
)

func TestHandlerProxiesRequestAndCapturesResponse(t *testing.T) {
	type seenRequest struct {
		method      string
		contentType string
		auth        string
		body        string
	}
	seen := make(chan seenRequest, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		seen <- seenRequest{
			method:      r.Method,
			contentType: r.Header.Get("Content-Type"),
			auth:        r.Header.Get("Authorization"),
			body:        string(body),
		}
		w.Header().Set("X-Trace-ID", "trace-1")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer target.Close()

	payload, err := json.Marshal(proxyRequest{
		Method: "post",
		URL:    target.URL + "/users",
		Headers: []headerValue{
			{Name: "Authorization", Value: "Bearer token"},
			{Name: "Content-Type", Value: "application/json"},
		},
		Body: `{"name":"fox"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	handler := Handler(Config{})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/__openapi-ui/request", bytes.NewReader(payload)))

	if rec.Code != http.StatusOK {
		t.Fatalf("proxy status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var result proxyResponse
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("proxy ok = false, error=%s", result.Error)
	}
	if result.StatusCode != http.StatusCreated {
		t.Fatalf("status code = %d, want %d", result.StatusCode, http.StatusCreated)
	}
	if got := result.Headers["X-Trace-Id"]; len(got) != 1 || got[0] != "trace-1" {
		t.Fatalf("response header X-Trace-Id = %#v", got)
	}
	if result.Body != `{"ok":true}` {
		t.Fatalf("body = %q", result.Body)
	}

	sent := <-seen
	if sent.method != http.MethodPost {
		t.Fatalf("target method = %s", sent.method)
	}
	if sent.auth != "Bearer token" {
		t.Fatalf("target authorization = %q", sent.auth)
	}
	if sent.contentType != "application/json" {
		t.Fatalf("target content type = %q", sent.contentType)
	}
	if sent.body != `{"name":"fox"}` {
		t.Fatalf("target body = %q", sent.body)
	}
}

func TestHandlerProxiesMultipartFile(t *testing.T) {
	seen := make(chan struct {
		filename    string
		contentType string
		body        string
		note        string
	}, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Errorf("parse multipart form: %v", err)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("form file: %v", err)
			return
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err != nil {
			t.Errorf("read file: %v", err)
			return
		}
		seen <- struct {
			filename    string
			contentType string
			body        string
			note        string
		}{
			filename:    header.Filename,
			contentType: header.Header.Get("Content-Type"),
			body:        string(body),
			note:        r.FormValue("note"),
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer target.Close()

	payload, err := json.Marshal(proxyRequest{
		Method: "post",
		URL:    target.URL + "/upload",
		Headers: []headerValue{
			{Name: "Content-Type", Value: "multipart/form-data"},
		},
		FormFields: []proxyFormField{{Name: "note", Value: "hello"}},
		Files: []proxyFile{{
			Name:        "file",
			Filename:    "hello.txt",
			ContentType: "text/plain",
			DataBase64:  base64.StdEncoding.EncodeToString([]byte("hello file")),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	handler := Handler(Config{})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/__openapi-ui/request", bytes.NewReader(payload)))

	if rec.Code != http.StatusOK {
		t.Fatalf("proxy status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	got := <-seen
	if got.filename != "hello.txt" {
		t.Fatalf("filename = %q, want hello.txt", got.filename)
	}
	if got.contentType != "text/plain" {
		t.Fatalf("content type = %q, want text/plain", got.contentType)
	}
	if got.body != "hello file" {
		t.Fatalf("file body = %q, want hello file", got.body)
	}
	if got.note != "hello" {
		t.Fatalf("note = %q, want hello", got.note)
	}
}

func TestHandlerRejectsOversizedMultipartFile(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("target should not be called")
	}))
	defer target.Close()

	payload, err := json.Marshal(proxyRequest{
		Method: "post",
		URL:    target.URL + "/upload",
		Files: []proxyFile{{
			Name:       "file",
			Filename:   "hello.txt",
			DataBase64: base64.StdEncoding.EncodeToString([]byte("hello file")),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	handler := Handler(Config{MaxProxyFileBytes: 4})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/__openapi-ui/request", bytes.NewReader(payload)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("proxy status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	var result proxyResponse
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Error, "exceeds limit") {
		t.Fatalf("error = %q, want limit error", result.Error)
	}
}

func TestHandlerRejectsOversizedProxyRequest(t *testing.T) {
	handler := Handler(Config{MaxProxyRequestBytes: 16})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/__openapi-ui/request", strings.NewReader(`{"method":"GET","url":"http://localhost:8080/health"}`)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("proxy status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandlerRejectsProxyToDisallowedHost(t *testing.T) {
	payload, err := json.Marshal(proxyRequest{
		Method: "GET",
		URL:    "http://169.254.169.254/latest/meta-data",
	})
	if err != nil {
		t.Fatal(err)
	}

	handler := Handler(Config{})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/__openapi-ui/request", bytes.NewReader(payload)))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("proxy status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	var result proxyResponse
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatal("proxy ok = true, want false")
	}
}

func TestHandlerRejectsRedirectToDisallowedHost(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data", http.StatusFound)
	}))
	defer target.Close()

	payload, err := json.Marshal(proxyRequest{
		Method: "GET",
		URL:    target.URL + "/redirect",
	})
	if err != nil {
		t.Fatal(err)
	}

	handler := Handler(Config{})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/__openapi-ui/request", bytes.NewReader(payload)))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("proxy status = %d, want %d; body=%s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	var result proxyResponse
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatal("proxy ok = true, want false")
	}
	if !strings.Contains(result.Error, "redirect target is not allowed") {
		t.Fatalf("error = %q, want redirect allowlist error", result.Error)
	}
}

func TestHandlerServesIndexAndAssets(t *testing.T) {
	handler := Handler(Config{
		Title:         "Docs",
		SpecURL:       "/openapi.json",
		StoragePrefix: "fox",
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("index status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"specUrl":"/openapi.json"`) {
		t.Fatalf("index body missing spec config: %s", body)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("Content-Security-Policy header is empty")
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want DENY", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/app.css", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("css status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
		t.Fatalf("css content type = %q, want text/css", got)
	}
}

func TestHandlerCORSPreflight(t *testing.T) {
	handler := Handler(Config{
		CORS: &CORSConfig{
			AllowedOrigins: []string{"https://example.com"},
			AllowedHeaders: []string{"Authorization"},
		},
	})

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want origin", got)
	}
}

func TestSpecHandler(t *testing.T) {
	doc := &openapi.Document{
		OpenAPI: "3.0.3",
		Info:    openapi.Info{Title: "API", Version: "1.0.0"},
		Paths:   openapi.Paths{},
	}
	rec := httptest.NewRecorder()
	SpecHandler(doc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("spec status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"openapi":"3.0.3"`) {
		t.Fatalf("spec body = %s, want openapi version", rec.Body.String())
	}
}

func TestFileSpecHandler(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "openapi-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"openapi":"3.0.3"}`); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	FileSpecHandler(file.Name()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("file spec status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"openapi":"3.0.3"}` {
		t.Fatalf("file spec body = %s", got)
	}
}
