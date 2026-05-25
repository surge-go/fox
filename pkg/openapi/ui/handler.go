package ui

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/surge-go/fox/pkg/openapi"
)

const (
	defaultMaxProxyRequestBytes  int64 = 64 << 20
	defaultMaxProxyResponseBytes int64 = 32 << 20
	defaultMaxProxyFileBytes     int64 = 32 << 20
)

type Config struct {
	Title                 string
	SpecURL               string
	CSP                   string
	CORS                  *CORSConfig
	StoragePrefix         string
	DisableProxy          bool
	ProxyHosts            []string
	MaxProxyRequestBytes  int64
	MaxProxyResponseBytes int64
	MaxProxyFileBytes     int64
}

type CORSConfig struct {
	AllowedOrigins []string
	AllowedHeaders []string
}

type proxyRequest struct {
	Method     string           `json:"method"`
	URL        string           `json:"url"`
	Headers    []headerValue    `json:"headers,omitempty"`
	Body       string           `json:"body,omitempty"`
	FormFields []proxyFormField `json:"formFields,omitempty"`
	Files      []proxyFile      `json:"files,omitempty"`
}

type headerValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type proxyFormField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type proxyFile struct {
	Name        string `json:"name"`
	Filename    string `json:"filename"`
	ContentType string `json:"contentType,omitempty"`
	DataBase64  string `json:"dataBase64"`
}

type proxyRequestEcho struct {
	Method      string           `json:"method"`
	URL         string           `json:"url"`
	Headers     []headerValue    `json:"headers,omitempty"`
	Body        string           `json:"body,omitempty"`
	FormFields  []proxyFormField `json:"formFields,omitempty"`
	Files       []proxyFile      `json:"files,omitempty"`
	ContentType string           `json:"contentType,omitempty"`
}

type proxyResponse struct {
	OK          bool                `json:"ok"`
	Method      string              `json:"method"`
	URL         string              `json:"url"`
	StatusCode  int                 `json:"statusCode"`
	Status      string              `json:"status"`
	DurationMS  int64               `json:"durationMs"`
	DurationUS  int64               `json:"durationUs,omitempty"`
	Bytes       int                 `json:"bytes"`
	ContentType string              `json:"contentType,omitempty"`
	Headers     map[string][]string `json:"headers,omitempty"`
	Body        string              `json:"body,omitempty"`
	Request     proxyRequestEcho    `json:"request"`
	Error       string              `json:"error,omitempty"`
}

type pageConfig struct {
	Title         string `json:"title"`
	SpecURL       string `json:"specUrl"`
	StoragePrefix string `json:"storagePrefix,omitempty"`
}

func Handler(cfg Config) http.Handler {
	if cfg.Title == "" {
		cfg.Title = "OpenAPI 工作台"
	}
	if cfg.SpecURL == "" {
		cfg.SpecURL = "openapi.json"
	}
	if cfg.CSP == "" {
		cfg.CSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/__openapi-ui/request", proxyRequestHandler(cfg))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		applySecurityHeaders(w, cfg)
		applyCORS(w, r, cfg.CORS)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "不支持的请求方法", http.StatusMethodNotAllowed)
			return
		}

		clean := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		switch clean {
		case "/", "/index.html":
			serveIndex(w, cfg)
		case "/app.css", "/app.js":
			serveAsset(w, clean[1:])
		default:
			http.NotFound(w, r)
		}
	})
	return mux
}

func SpecHandler(doc *openapi.Document) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "不支持的请求方法", http.StatusMethodNotAllowed)
			return
		}
		if doc == nil {
			http.Error(w, "文档为空", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if err := json.NewEncoder(w).Encode(doc); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}

func proxyRequestHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		applySecurityHeaders(w, cfg)
		applyCORS(w, r, cfg.CORS)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "不支持的请求方法", http.StatusMethodNotAllowed)
			return
		}

		var payload proxyRequest
		requestLimit := proxyRequestLimit(cfg)
		if requestLimit > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, requestLimit)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeProxyJSON(w, http.StatusBadRequest, proxyResponse{
				OK:     false,
				Status: http.StatusText(http.StatusBadRequest),
				Error:  err.Error(),
			})
			return
		}

		payload.Method = strings.ToUpper(strings.TrimSpace(payload.Method))
		payload.URL = strings.TrimSpace(payload.URL)
		if payload.Method == "" {
			writeProxyJSON(w, http.StatusBadRequest, proxyResponse{
				OK:     false,
				Status: http.StatusText(http.StatusBadRequest),
				Error:  "请求方法不能为空",
			})
			return
		}
		if payload.URL == "" {
			writeProxyJSON(w, http.StatusBadRequest, proxyResponse{
				OK:     false,
				Status: http.StatusText(http.StatusBadRequest),
				Error:  "请求地址不能为空",
			})
			return
		}
		parsedURL, err := url.Parse(payload.URL)
		if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
			writeProxyJSON(w, http.StatusBadRequest, proxyResponse{
				OK:     false,
				Status: http.StatusText(http.StatusBadRequest),
				Error:  "无效的请求地址",
			})
			return
		}
		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			writeProxyJSON(w, http.StatusBadRequest, proxyResponse{
				OK:     false,
				Status: http.StatusText(http.StatusBadRequest),
				Error:  "仅支持 http 和 https 请求地址",
			})
			return
		}
		if cfg.DisableProxy {
			writeProxyJSON(w, http.StatusForbidden, proxyResponse{
				OK:     false,
				Status: http.StatusText(http.StatusForbidden),
				Error:  "UI 请求代理已关闭",
			})
			return
		}
		if !proxyHostAllowed(parsedURL, r, cfg.ProxyHosts) {
			writeProxyJSON(w, http.StatusForbidden, proxyResponse{
				OK:     false,
				Status: http.StatusText(http.StatusForbidden),
				Error:  "请求地址不在 UI 代理允许范围内",
			})
			return
		}

		req, multipartContentType, err := newProxyHTTPRequest(r.Context(), payload, proxyFileLimit(cfg))
		if err != nil {
			writeProxyJSON(w, http.StatusBadRequest, proxyResponse{
				OK:     false,
				Status: http.StatusText(http.StatusBadRequest),
				Error:  err.Error(),
			})
			return
		}

		for _, header := range payload.Headers {
			name := strings.TrimSpace(header.Name)
			if name == "" {
				continue
			}
			req.Header.Set(http.CanonicalHeaderKey(name), header.Value)
		}
		if multipartContentType != "" {
			req.Header.Set("Content-Type", multipartContentType)
		}
		if payload.Body != "" && len(payload.Files) == 0 && req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if req.Header.Get("Accept") == "" {
			req.Header.Set("Accept", "application/json, text/plain, */*")
		}

		client := &http.Client{
			Timeout:       30 * time.Second,
			CheckRedirect: proxyRedirectPolicy(r, cfg.ProxyHosts),
		}
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			statusCode := http.StatusBadGateway
			if errors.Is(err, context.DeadlineExceeded) {
				statusCode = http.StatusGatewayTimeout
			}
			durationMS, durationUS := elapsedDurations(start)
			writeProxyJSON(w, statusCode, proxyResponse{
				OK:         false,
				Method:     payload.Method,
				URL:        payload.URL,
				StatusCode: statusCode,
				Status:     http.StatusText(statusCode),
				DurationMS: durationMS,
				DurationUS: durationUS,
				Error:      err.Error(),
				Request:    echoRequest(payload),
			})
			return
		}
		defer resp.Body.Close()

		responseBody, err := readLimited(resp.Body, proxyResponseLimit(cfg))
		if err != nil {
			durationMS, durationUS := elapsedDurations(start)
			writeProxyJSON(w, http.StatusBadGateway, proxyResponse{
				OK:         false,
				Method:     payload.Method,
				URL:        payload.URL,
				StatusCode: http.StatusBadGateway,
				Status:     http.StatusText(http.StatusBadGateway),
				DurationMS: durationMS,
				DurationUS: durationUS,
				Error:      err.Error(),
				Request:    echoRequest(payload),
			})
			return
		}

		durationMS, durationUS := elapsedDurations(start)
		writeProxyJSON(w, http.StatusOK, proxyResponse{
			OK:          resp.StatusCode >= 200 && resp.StatusCode < 400,
			Method:      payload.Method,
			URL:         payload.URL,
			StatusCode:  resp.StatusCode,
			Status:      resp.Status,
			DurationMS:  durationMS,
			DurationUS:  durationUS,
			Bytes:       len(responseBody),
			ContentType: resp.Header.Get("Content-Type"),
			Headers:     cloneHeader(resp.Header),
			Body:        string(responseBody),
			Request:     echoRequest(payload),
		})
	}
}

func proxyRequestLimit(cfg Config) int64 {
	if cfg.MaxProxyRequestBytes > 0 {
		return cfg.MaxProxyRequestBytes
	}
	return defaultMaxProxyRequestBytes
}

func proxyResponseLimit(cfg Config) int64 {
	if cfg.MaxProxyResponseBytes > 0 {
		return cfg.MaxProxyResponseBytes
	}
	return defaultMaxProxyResponseBytes
}

func proxyFileLimit(cfg Config) int64 {
	if cfg.MaxProxyFileBytes > 0 {
		return cfg.MaxProxyFileBytes
	}
	return defaultMaxProxyFileBytes
}

func proxyRedirectPolicy(original *http.Request, allowedHosts []string) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if !proxyHostAllowed(req.URL, original, allowedHosts) {
			return fmt.Errorf("redirect target is not allowed: %s", req.URL.String())
		}
		return nil
	}
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return io.ReadAll(r)
	}
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response body exceeds limit: %d bytes", limit)
	}
	return body, nil
}

func elapsedDurations(start time.Time) (int64, int64) {
	elapsed := time.Since(start)
	if elapsed <= 0 {
		return 0, 0
	}
	return elapsed.Milliseconds(), elapsed.Microseconds()
}

func newProxyHTTPRequest(ctx context.Context, payload proxyRequest, maxFileBytes int64) (*http.Request, string, error) {
	if len(payload.Files) == 0 && len(payload.FormFields) == 0 {
		var body io.Reader
		if payload.Body != "" {
			body = bytes.NewBufferString(payload.Body)
		}
		req, err := http.NewRequestWithContext(ctx, payload.Method, payload.URL, body)
		return req, "", err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, field := range payload.FormFields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			continue
		}
		if err := writer.WriteField(name, field.Value); err != nil {
			return nil, "", err
		}
	}
	for _, file := range payload.Files {
		name := strings.TrimSpace(file.Name)
		if name == "" {
			continue
		}
		filename := strings.TrimSpace(file.Filename)
		if filename == "" {
			filename = "upload"
		}
		if maxFileBytes > 0 && int64(len(file.DataBase64)) > encodedBase64Limit(maxFileBytes) {
			return nil, "", fmt.Errorf("file %q exceeds limit: %d bytes", filename, maxFileBytes)
		}
		data, err := base64.StdEncoding.DecodeString(file.DataBase64)
		if err != nil {
			return nil, "", err
		}
		if maxFileBytes > 0 && int64(len(data)) > maxFileBytes {
			return nil, "", fmt.Errorf("file %q exceeds limit: %d bytes", filename, maxFileBytes)
		}
		part, err := createMultipartFilePart(writer, name, filename, file.ContentType)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(data); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, payload.Method, payload.URL, &body)
	if err != nil {
		return nil, "", err
	}
	return req, writer.FormDataContentType(), nil
}

func encodedBase64Limit(decodedBytes int64) int64 {
	return ((decodedBytes + 2) / 3) * 4
}

func createMultipartFilePart(writer *multipart.Writer, fieldName, filename, contentType string) (io.Writer, error) {
	if strings.TrimSpace(contentType) == "" {
		return writer.CreateFormFile(fieldName, filename)
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeMultipartQuote(fieldName), escapeMultipartQuote(filename)))
	header.Set("Content-Type", contentType)
	return writer.CreatePart(header)
}

func escapeMultipartQuote(value string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(value)
}

func FileSpecHandler(filePath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "不支持的请求方法", http.StatusMethodNotAllowed)
			return
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Write(data)
	})
}

func writeProxyJSON(w http.ResponseWriter, status int, payload proxyResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func serveIndex(w http.ResponseWriter, cfg Config) {
	data, err := assets.ReadFile("index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	payload, err := json.Marshal(pageConfig{
		Title:         cfg.Title,
		SpecURL:       cfg.SpecURL,
		StoragePrefix: cfg.StoragePrefix,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	html := strings.ReplaceAll(string(data), `{"title":"OpenAPI 工作台","specUrl":"openapi.json","storagePrefix":"openapi-ui"}`, string(payload))
	html = strings.ReplaceAll(html, "OpenAPI 工作台", htmlEscape(cfg.Title))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(html))
}

func serveAsset(w http.ResponseWriter, name string) {
	data, err := assets.ReadFile(name)
	if err != nil {
		http.Error(w, "资源不存在", http.StatusNotFound)
		return
	}
	switch path.Ext(name) {
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	default:
		w.Header().Set("Content-Type", http.DetectContentType(data))
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
}

func applySecurityHeaders(w http.ResponseWriter, cfg Config) {
	w.Header().Set("Content-Security-Policy", cfg.CSP)
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func applyCORS(w http.ResponseWriter, r *http.Request, cfg *CORSConfig) {
	if cfg == nil {
		return
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}
	for _, allowed := range cfg.AllowedOrigins {
		if allowed == "*" || allowed == origin {
			w.Header().Set("Access-Control-Allow-Origin", allowed)
			w.Header().Set("Vary", "Origin")
			if len(cfg.AllowedHeaders) > 0 {
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ", "))
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			return
		}
	}
}

func proxyHostAllowed(target *url.URL, r *http.Request, allowedHosts []string) bool {
	host := strings.ToLower(target.Hostname())
	if host == "" {
		return false
	}
	for _, allowed := range allowedHosts {
		allowed = strings.ToLower(strings.TrimSpace(allowed))
		if allowed == "" {
			continue
		}
		if allowed == "*" || allowed == host || allowed == strings.ToLower(target.Host) {
			return true
		}
	}
	if requestHost := strings.ToLower(requestHostname(r)); requestHost != "" && host == requestHost {
		return true
	}
	return isLoopbackHost(host)
}

func requestHostname(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := r.Host
	if host == "" && r.URL != nil {
		host = r.URL.Host
	}
	if strings.Contains(host, ":") {
		if parsed, err := url.Parse("//" + host); err == nil {
			return parsed.Hostname()
		}
	}
	return host
}

func isLoopbackHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	default:
		return strings.HasPrefix(host, "127.")
	}
}

func cloneHeader(src http.Header) map[string][]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string][]string, len(src))
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
	return dst
}

func echoRequest(payload proxyRequest) proxyRequestEcho {
	return proxyRequestEcho{
		Method:      payload.Method,
		URL:         payload.URL,
		Headers:     payload.Headers,
		Body:        payload.Body,
		FormFields:  payload.FormFields,
		Files:       echoFiles(payload.Files),
		ContentType: headerValueFor(payload.Headers, "Content-Type"),
	}
}

func echoFiles(files []proxyFile) []proxyFile {
	if len(files) == 0 {
		return nil
	}
	out := make([]proxyFile, 0, len(files))
	for _, file := range files {
		out = append(out, proxyFile{
			Name:        file.Name,
			Filename:    file.Filename,
			ContentType: file.ContentType,
		})
	}
	return out
}

func headerValueFor(headers []headerValue, name string) string {
	for _, header := range headers {
		if strings.EqualFold(strings.TrimSpace(header.Name), name) {
			return header.Value
		}
	}
	return ""
}

func htmlEscape(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&#34;",
		"'", "&#39;",
	)
	return replacer.Replace(value)
}
