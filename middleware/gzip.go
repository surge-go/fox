package middleware

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/surge-go/fox"
)

const (
	defaultGzipLevel   = gzip.DefaultCompression
	defaultGzipMinSize = 1024

	gzipEncoding = "gzip"
)

var (
	defaultGzipContentTypes = []string{
		"text/plain",
		"text/html",
		"text/css",
		"text/javascript",
		"text/xml",
		"application/json",
		"application/javascript",
		"application/xml",
		"application/rss+xml",
		"application/atom+xml",
		"application/svg+xml",
	}

	defaultGzipExcludedExtensions = []string{
		".7z",
		".avi",
		".br",
		".bz2",
		".gif",
		".gz",
		".jpeg",
		".jpg",
		".mp3",
		".mp4",
		".pdf",
		".png",
		".rar",
		".webp",
		".zip",
	}
)

// GzipSkipFunc 判断当前请求是否跳过 gzip 压缩。
type GzipSkipFunc func(*fox.Context) bool

// GzipConfig 表示 gzip 中间件配置。
type GzipConfig struct {
	// Level 表示 gzip 压缩等级；0 使用默认等级。
	Level int
	// MinSize 表示触发压缩的最小响应体字节数；0 使用默认值，负数表示不设阈值。
	MinSize int
	// ContentTypes 表示允许压缩的响应 Content-Type，空值使用默认文本类型。
	ContentTypes []string
	// ExcludedExtensions 表示不压缩的请求路径后缀，空值使用默认二进制/已压缩类型。
	ExcludedExtensions []string
	// SkipPaths 表示跳过 gzip 的请求路径。
	SkipPaths []string
	// SkipFunc 判断是否跳过 gzip。
	SkipFunc GzipSkipFunc
}

// DefaultGzipConfig 返回 gzip 中间件默认配置。
func DefaultGzipConfig() GzipConfig {
	return GzipConfig{
		Level:              defaultGzipLevel,
		MinSize:            defaultGzipMinSize,
		ContentTypes:       append([]string(nil), defaultGzipContentTypes...),
		ExcludedExtensions: append([]string(nil), defaultGzipExcludedExtensions...),
	}
}

// Gzip 返回使用默认配置的 gzip 压缩中间件。
func Gzip() fox.HandlerFunc {
	return GzipWithConfig(DefaultGzipConfig())
}

// GzipWithConfig 返回使用自定义配置的 gzip 压缩中间件。
func GzipWithConfig(cfg GzipConfig) fox.HandlerFunc {
	useDefaultContentTypes := len(cfg.ContentTypes) == 0
	cfg = normalizeGzipConfig(cfg)
	skipPaths := makeGzipSkipPaths(cfg.SkipPaths)
	contentTypes := makeGzipContentTypes(cfg.ContentTypes)
	excludedExtensions := makeGzipExcludedExtensions(cfg.ExcludedExtensions)

	return func(c *fox.Context) {
		if shouldSkipGzip(c, skipPaths, excludedExtensions, cfg.SkipFunc) || !requestAcceptsGzip(c.RawRequest()) {
			c.Next()
			return
		}

		originalWriter := c.RawWriter().(gin.ResponseWriter)
		writer := &gzipResponseWriter{
			ResponseWriter: originalWriter,
			level:          cfg.Level,
			minSize:        cfg.MinSize,
			contentTypes:   contentTypes,
			allowSuffixes:  useDefaultContentTypes,
		}
		c.SetRawWriter(writer)
		defer func() {
			recovered := recover()
			if recovered != nil {
				if writer.started {
					_ = writer.Close()
				}
				c.SetRawWriter(originalWriter)
				panic(recovered)
			}
			_ = writer.Close()
			c.SetRawWriter(originalWriter)
		}()
		c.Next()
	}
}

func normalizeGzipConfig(cfg GzipConfig) GzipConfig {
	defaults := DefaultGzipConfig()
	if cfg.Level == 0 {
		cfg.Level = defaults.Level
	}
	if cfg.Level < gzip.HuffmanOnly || cfg.Level > gzip.BestCompression {
		cfg.Level = defaults.Level
	}
	if cfg.MinSize == 0 {
		cfg.MinSize = defaults.MinSize
	}
	if cfg.MinSize < 0 {
		cfg.MinSize = 0
	}
	if len(cfg.ContentTypes) == 0 {
		cfg.ContentTypes = defaults.ContentTypes
	}
	if len(cfg.ExcludedExtensions) == 0 {
		cfg.ExcludedExtensions = defaults.ExcludedExtensions
	}
	return cfg
}

func makeGzipSkipPaths(paths []string) map[string]struct{} {
	if len(paths) == 0 {
		return nil
	}
	skipPaths := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			skipPaths[path] = struct{}{}
		}
	}
	return skipPaths
}

func makeGzipContentTypes(contentTypes []string) map[string]struct{} {
	allowed := make(map[string]struct{}, len(contentTypes))
	for _, contentType := range contentTypes {
		contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
		if contentType != "" {
			allowed[contentType] = struct{}{}
		}
	}
	return allowed
}

func makeGzipExcludedExtensions(extensions []string) map[string]struct{} {
	excluded := make(map[string]struct{}, len(extensions))
	for _, extension := range extensions {
		extension = strings.ToLower(strings.TrimSpace(extension))
		if extension == "" {
			continue
		}
		if !strings.HasPrefix(extension, ".") {
			extension = "." + extension
		}
		excluded[extension] = struct{}{}
	}
	return excluded
}

func shouldSkipGzip(c *fox.Context, skipPaths map[string]struct{}, excludedExtensions map[string]struct{}, skipFunc GzipSkipFunc) bool {
	if skipFunc != nil && skipFunc(c) {
		return true
	}
	path := c.RawRequest().URL.Path
	if _, ok := skipPaths[path]; ok {
		return true
	}
	if len(excludedExtensions) == 0 {
		return false
	}
	_, ok := excludedExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
}

func requestAcceptsGzip(req *http.Request) bool {
	if req.Method == http.MethodHead {
		return false
	}
	wildcardQuality := 0.0
	for _, value := range strings.Split(req.Header.Get("Accept-Encoding"), ",") {
		parts := strings.Split(value, ";")
		encoding := strings.TrimSpace(parts[0])
		if strings.EqualFold(encoding, gzipEncoding) {
			return gzipEncodingQuality(parts[1:]) > 0
		}
		if encoding == "*" {
			wildcardQuality = gzipEncodingQuality(parts[1:])
		}
	}
	return wildcardQuality > 0
}

func gzipEncodingQuality(params []string) float64 {
	for _, param := range params {
		name, value, ok := strings.Cut(strings.TrimSpace(param), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "q") {
			continue
		}
		quality, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 1
		}
		return quality
	}
	return 1
}

type gzipResponseWriter struct {
	gin.ResponseWriter
	level         int
	minSize       int
	contentTypes  map[string]struct{}
	allowSuffixes bool

	status         int
	buffer         bytes.Buffer
	gzipWriter     *gzip.Writer
	started        bool
	logicalWritten bool
	closed         bool
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	if w.Written() || w.started {
		return
	}
	w.status = status
}

func (w *gzipResponseWriter) WriteHeaderNow() {
	if w.Written() || w.started {
		return
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.logicalWritten = true
}

func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if w.closed {
		return 0, errors.New("fox middleware: gzip response writer is closed")
	}
	if w.started {
		if !gzipStatusAllowsBody(w.status) {
			return len(data), nil
		}
		if w.gzipWriter != nil {
			if _, err := w.gzipWriter.Write(data); err != nil {
				return 0, err
			}
			return len(data), nil
		}
		return w.ResponseWriter.Write(data)
	}

	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.logicalWritten = true
	w.buffer.Write(data)
	if w.buffer.Len() >= w.minSize {
		if w.shouldCompressBuffered() {
			w.startCompress()
			return len(data), nil
		}
		w.startPlain()
		return len(data), nil
	}
	return len(data), nil
}

func (w *gzipResponseWriter) WriteString(value string) (int, error) {
	return w.Write([]byte(value))
}

func (w *gzipResponseWriter) Status() int {
	if w.status != 0 {
		return w.status
	}
	return w.ResponseWriter.Status()
}

func (w *gzipResponseWriter) Size() int {
	if w.started {
		return w.ResponseWriter.Size()
	}
	if w.buffer.Len() > 0 {
		return w.buffer.Len()
	}
	if w.logicalWritten {
		return 0
	}
	return w.ResponseWriter.Size()
}

func (w *gzipResponseWriter) Written() bool {
	return w.logicalWritten || (w.started && w.ResponseWriter.Written())
}

func (w *gzipResponseWriter) Flush() {
	if !w.started {
		if w.shouldCompressBuffered() {
			w.startCompress()
		} else {
			w.startPlain()
		}
	}
	if w.gzipWriter != nil {
		_ = w.gzipWriter.Flush()
	}
	w.ResponseWriter.Flush()
}

func (w *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.Hijack()
}

func (w *gzipResponseWriter) CloseNotify() <-chan bool {
	return w.ResponseWriter.CloseNotify()
}

func (w *gzipResponseWriter) Pusher() http.Pusher {
	return w.ResponseWriter.Pusher()
}

func (w *gzipResponseWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if !w.started {
		if w.buffer.Len() >= w.minSize && w.shouldCompressBuffered() {
			w.startCompress()
		} else {
			w.startPlain()
		}
	}
	if w.gzipWriter != nil {
		return w.gzipWriter.Close()
	}
	return nil
}

func (w *gzipResponseWriter) startCompress() {
	if w.started {
		return
	}
	w.started = true
	header := w.Header()
	header.Del("Content-Length")
	header.Set("Content-Encoding", gzipEncoding)
	addVaryHeaderToHTTP(header, "Accept-Encoding")
	w.writeHeader()
	gzipWriter, err := gzip.NewWriterLevel(w.ResponseWriter, w.level)
	if err != nil {
		gzipWriter = gzip.NewWriter(w.ResponseWriter)
	}
	w.gzipWriter = gzipWriter
	if w.buffer.Len() > 0 {
		_, _ = io.Copy(w.gzipWriter, &w.buffer)
	}
}

func (w *gzipResponseWriter) startPlain() {
	if w.started {
		return
	}
	w.started = true
	w.writeHeader()
	if !gzipStatusAllowsBody(w.status) {
		w.buffer.Reset()
		return
	}
	if w.buffer.Len() > 0 {
		_, _ = io.Copy(w.ResponseWriter, &w.buffer)
	}
}

func (w *gzipResponseWriter) writeHeader() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.logicalWritten = true
	w.ResponseWriter.WriteHeader(w.status)
}

func (w *gzipResponseWriter) shouldCompressBuffered() bool {
	header := w.Header()
	if header.Get("Content-Encoding") != "" {
		return false
	}
	if strings.Contains(strings.ToLower(header.Get("Cache-Control")), "no-transform") {
		return false
	}
	if !gzipStatusAllowsBody(w.status) {
		return false
	}
	contentType := header.Get("Content-Type")
	if contentType == "" && w.buffer.Len() > 0 {
		contentType = http.DetectContentType(w.buffer.Bytes())
		header.Set("Content-Type", contentType)
	}
	if strings.HasPrefix(strings.ToLower(contentType), "text/event-stream") {
		return false
	}
	return gzipContentTypeAllowed(contentType, w.contentTypes, w.allowSuffixes)
}

func gzipStatusAllowsBody(status int) bool {
	if status == 0 {
		return true
	}
	if status >= 100 && status < 200 {
		return false
	}
	return status != http.StatusNoContent && status != http.StatusNotModified
}

func gzipContentTypeAllowed(contentType string, allowed map[string]struct{}, allowSuffixes bool) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if contentType == "" {
		return false
	}
	if _, ok := allowed[contentType]; ok {
		return true
	}
	return allowSuffixes && (strings.HasSuffix(contentType, "+json") || strings.HasSuffix(contentType, "+xml"))
}

func addVaryHeaderToHTTP(header http.Header, value string) {
	current := header.Get("Vary")
	if current == "" {
		header.Set("Vary", value)
		return
	}
	for _, item := range strings.Split(current, ",") {
		if strings.EqualFold(strings.TrimSpace(item), value) {
			return
		}
	}
	header.Set("Vary", current+", "+value)
}
