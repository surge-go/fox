package fox

import (
	"context"
	stderrors "errors"
	"io/fs"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/surge-go/fox/core/errors"
)

// Context 封装 gin.Context，并实现标准库 context.Context 接口。
type Context struct {
	ctx    *gin.Context
	errors IErrors
}

// StdContext 返回标准库 context.Context。
func (c *Context) StdContext() context.Context { return c.ctx.Request.Context() }

// RawRequest 返回底层 *http.Request。
func (c *Context) RawRequest() *http.Request { return c.ctx.Request }

// RawWriter 返回底层 http.ResponseWriter。
func (c *Context) RawWriter() http.ResponseWriter {
	return c.ctx.Writer
}

// SetRawWriter 替换底层响应写入器。
//
// writer 必须实现 Gin 兼容的 ResponseWriter；该方法主要供需要包装响应写入器的中间件使用。
func (c *Context) SetRawWriter(writer http.ResponseWriter) {
	if writer == nil {
		return
	}
	ginWriter, ok := writer.(gin.ResponseWriter)
	if !ok {
		panic("fox: raw writer must implement gin.ResponseWriter")
	}
	c.ctx.Writer = ginWriter
}

// WithContext 替换当前请求携带的标准库 context.Context。
func (c *Context) WithContext(ctx context.Context) {
	if ctx != nil {
		c.ctx.Request = c.ctx.Request.WithContext(ctx)
	}
}

// Status 返回当前响应状态码。
func (c *Context) Status() int { return c.ctx.Writer.Status() }

// Written 判断响应头或响应体是否已经写入。
func (c *Context) Written() bool { return c.ctx.Writer.Written() }

// WithValue 向请求 context 写入键值对。
func (c *Context) WithValue(key, value any) {
	c.WithContext(context.WithValue(c.StdContext(), key, value))
}

// Deadline 实现 context.Context 接口。
func (c *Context) Deadline() (time.Time, bool) { return c.StdContext().Deadline() }

// Done 实现 context.Context 接口。
func (c *Context) Done() <-chan struct{} { return c.StdContext().Done() }

// Err 实现 context.Context 接口。
func (c *Context) Err() error { return c.StdContext().Err() }

// Value 实现 context.Context 接口。
func (c *Context) Value(key any) any { return c.StdContext().Value(key) }

// SetRequestID 设置当前请求的 request id。
func (c *Context) SetRequestID(id string) { c.Set(RequestIDKey, id) }

// RequestID 返回当前请求的 request id。
func (c *Context) RequestID() string { return c.GetString(RequestIDKey) }

// SetTraceID 设置当前请求的 trace id。
func (c *Context) SetTraceID(id string) { c.Set(TraceIDKey, id) }

// TraceID 返回当前请求的 trace id。
func (c *Context) TraceID() string { return c.GetString(TraceIDKey) }

// SetSpanID 设置当前请求的 span id。
func (c *Context) SetSpanID(id string) { c.Set(SpanIDKey, id) }

// SpanID 返回当前请求的 span id。
func (c *Context) SpanID() string { return c.GetString(SpanIDKey) }

// Errors 返回当前请求所属 Engine 的错误工厂。
func (c *Context) Errors() IErrors { return c.errorFactory() }

// Copy 创建可在 goroutine 中使用的 Context 副本。
func (c *Context) Copy() *Context {
	return &Context{ctx: c.ctx.Copy(), errors: c.errors}
}

// Next 执行处理链中的后续处理函数。
func (c *Context) Next() { c.ctx.Next() }

// FullPath 返回匹配到的路由模板路径。
func (c *Context) FullPath() string { return c.ctx.FullPath() }

// Abort 中止当前请求的后续处理。
func (c *Context) Abort() { c.ctx.Abort() }

// IsAborted 判断当前请求是否已被中止。
func (c *Context) IsAborted() bool { return c.ctx.IsAborted() }

// AbortWithStatus 中止请求并写入 HTTP 状态码。
func (c *Context) AbortWithStatus(status int) { c.ctx.AbortWithStatus(status) }

// AbortWithStatusJSON 中止请求并写入 JSON 响应体。
func (c *Context) AbortWithStatusJSON(status int, data any) { c.ctx.AbortWithStatusJSON(status, data) }

// Set 在请求上下文中保存键值对。
func (c *Context) Set(key string, value any) { c.ctx.Set(key, value) }

// Get 从请求上下文中读取键值对。
func (c *Context) Get(key string) (any, bool) { return c.ctx.Get(key) }

// GetString 从请求上下文中读取 string 值。
func (c *Context) GetString(key string) string { return c.ctx.GetString(key) }

// GetBool 从请求上下文中读取 bool 值。
func (c *Context) GetBool(key string) bool { return c.ctx.GetBool(key) }

// GetInt 从请求上下文中读取 int 值。
func (c *Context) GetInt(key string) int { return c.ctx.GetInt(key) }

// GetInt64 从请求上下文中读取 int64 值。
func (c *Context) GetInt64(key string) int64 { return c.ctx.GetInt64(key) }

// GetUint 从请求上下文中读取 uint 值。
func (c *Context) GetUint(key string) uint { return c.ctx.GetUint(key) }

// GetUint64 从请求上下文中读取 uint64 值。
func (c *Context) GetUint64(key string) uint64 { return c.ctx.GetUint64(key) }

// GetFloat64 从请求上下文中读取 float64 值。
func (c *Context) GetFloat64(key string) float64 { return c.ctx.GetFloat64(key) }

// GetTime 从请求上下文中读取 time.Time 值。
func (c *Context) GetTime(key string) time.Time { return c.ctx.GetTime(key) }

// GetDuration 从请求上下文中读取 time.Duration 值。
func (c *Context) GetDuration(key string) time.Duration { return c.ctx.GetDuration(key) }

// GetStringSlice 从请求上下文中读取 []string 值。
func (c *Context) GetStringSlice(key string) []string { return c.ctx.GetStringSlice(key) }

// GetStringMap 从请求上下文中读取 map[string]any 值。
func (c *Context) GetStringMap(key string) map[string]any {
	return c.ctx.GetStringMap(key)
}

// GetStringMapString 从请求上下文中读取 map[string]string 值。
func (c *Context) GetStringMapString(key string) map[string]string {
	return c.ctx.GetStringMapString(key)
}

// GetStringMapStringSlice 从请求上下文中读取 map[string][]string 值。
func (c *Context) GetStringMapStringSlice(key string) map[string][]string {
	return c.ctx.GetStringMapStringSlice(key)
}

// MustGet 从请求上下文中读取值；不存在时 panic。
func (c *Context) MustGet(key string) any { return c.ctx.MustGet(key) }

// Delete 从请求上下文中删除键值对。
func (c *Context) Delete(key string) { c.ctx.Delete(key) }

// Bind 根据请求 Content-Type 自动绑定请求参数。
func (c *Context) Bind(obj any) error { return c.bindOrFail(c.ctx.ShouldBind(obj)) }

// BindJSON 绑定 JSON 请求体。
func (c *Context) BindJSON(obj any) error { return c.bindOrFail(c.ctx.ShouldBindJSON(obj)) }

// BindQuery 绑定 URL query 参数。
func (c *Context) BindQuery(obj any) error { return c.bindOrFail(c.ctx.ShouldBindQuery(obj)) }

// BindURI 绑定路由路径参数。
func (c *Context) BindURI(obj any) error { return c.bindOrFail(c.ctx.ShouldBindUri(obj)) }

// bindOrFail 统一处理绑定错误，失败时写入标准错误响应并中止请求。
func (c *Context) bindOrFail(err error) error {
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if stderrors.As(err, &maxBytesErr) {
			c.writeErrorResponse(c.errorFactory().ErrPayloadTooLarge(), c.TraceID())
			return err
		}
		c.Fail(c.errorFactory().ErrInvalidParams())
	}
	return err
}

// Param 返回路由路径参数。
func (c *Context) Param(key string) string { return c.ctx.Param(key) }

// Query 返回 URL query 参数。
func (c *Context) Query(key string) string { return c.ctx.Query(key) }

// DefaultQuery 返回 URL query 参数；不存在时返回默认值。
func (c *Context) DefaultQuery(key, defaultValue string) string {
	return c.ctx.DefaultQuery(key, defaultValue)
}

// GetQuery 返回 URL query 参数及其是否存在。
func (c *Context) GetQuery(key string) (string, bool) { return c.ctx.GetQuery(key) }

// QueryArray 返回同名 URL query 参数列表。
func (c *Context) QueryArray(key string) []string { return c.ctx.QueryArray(key) }

// QueryMap 返回 map 形式的 URL query 参数。
func (c *Context) QueryMap(key string) map[string]string { return c.ctx.QueryMap(key) }

// PostForm 返回 POST 表单参数。
func (c *Context) PostForm(key string) string { return c.ctx.PostForm(key) }

// DefaultPostForm 返回 POST 表单参数；不存在时返回默认值。
func (c *Context) DefaultPostForm(key, defaultValue string) string {
	return c.ctx.DefaultPostForm(key, defaultValue)
}

// GetPostForm 返回 POST 表单参数及其是否存在。
func (c *Context) GetPostForm(key string) (string, bool) { return c.ctx.GetPostForm(key) }

// PostFormArray 返回同名 POST 表单参数列表。
func (c *Context) PostFormArray(key string) []string { return c.ctx.PostFormArray(key) }

// PostFormMap 返回 map 形式的 POST 表单参数。
func (c *Context) PostFormMap(key string) map[string]string { return c.ctx.PostFormMap(key) }

// FormFile 返回上传文件头。
func (c *Context) FormFile(key string) (*multipart.FileHeader, error) {
	return c.ctx.FormFile(key)
}

// MultipartForm 返回 multipart 表单。
func (c *Context) MultipartForm() (*multipart.Form, error) { return c.ctx.MultipartForm() }

// SaveUploadedFile 保存上传文件。
func (c *Context) SaveUploadedFile(file *multipart.FileHeader, dst string, perm ...fs.FileMode) error {
	return c.ctx.SaveUploadedFile(file, dst, perm...)
}

// ClientIP 返回客户端 IP。
func (c *Context) ClientIP() string { return c.ctx.ClientIP() }

// ContentType 返回请求 Content-Type。
func (c *Context) ContentType() string { return c.ctx.ContentType() }

// GetHeader 返回请求头。
func (c *Context) GetHeader(key string) string { return c.ctx.GetHeader(key) }

// GetCookie 返回请求 Cookie。
func (c *Context) GetCookie(name string) (string, error) { return c.ctx.Cookie(name) }

// SetHeader 设置响应头。
func (c *Context) SetHeader(key, value string) { c.ctx.Header(key, value) }

// GetResponseHeader 返回响应头。
func (c *Context) GetResponseHeader(key string) string { return c.ctx.Writer.Header().Get(key) }

// SetCookie 设置响应 Cookie。
func (c *Context) SetCookie(name, value string, maxAge int, path, domain string, secure, httpOnly bool) {
	c.ctx.SetCookie(name, value, maxAge, path, domain, secure, httpOnly)
}

// JSON 写入 JSON 响应。
func (c *Context) JSON(status int, data any) { c.ctx.JSON(status, data) }

// HTML 写入 HTML 响应。
func (c *Context) HTML(status int, name string, data any) { c.ctx.HTML(status, name, data) }

// String 写入纯文本响应。
func (c *Context) String(status int, text string) { c.ctx.String(status, text) }

// File 写入文件响应。
func (c *Context) File(path string) { c.ctx.File(path) }

// Redirect 写入重定向响应。
func (c *Context) Redirect(status int, path string) { c.ctx.Redirect(status, path) }

// Ok 写入成功响应。
func (c *Context) Ok(data any) {
	traceID := c.TraceID()
	c.JSON(http.StatusOK, NewResponse(200, data, "success", traceID))
	c.Abort()
}

// Fail 写入错误响应。
//
// 如果 err 是 core/errors.Error，会使用其中的 HTTP 状态码、业务码和公开消息；
// 其它错误统一隐藏为 500 internal server error。
func (c *Context) Fail(err error) {
	if err == nil {
		c.writeErrorResponse(c.errorFactory().ErrServer(), c.TraceID())
		return
	}

	traceID := c.TraceID()
	if e, ok := errors.As(err); ok {
		c.writeErrorResponse(e, traceID)
		return
	}

	c.writeErrorResponse(c.errorFactory().ErrServer(), traceID)
}

// validHTTPStatusOrDefault 兜底修正非法 HTTP 状态码。
func validHTTPStatusOrDefault(status int) int {
	if status < 100 || status > 599 {
		return http.StatusInternalServerError
	}
	return status
}

func (c *Context) writeErrorResponse(err *errors.Error, traceID string) {
	if err == nil {
		err = defaultErrorFactory().ErrServer()
	}
	c.JSON(validHTTPStatusOrDefault(err.Status()), NewResponse(err.Code, nil, err.Message, traceID))
	c.Abort()
}

func (c *Context) errorFactory() IErrors {
	if c != nil && c.errors != nil {
		return c.errors
	}
	return defaultErrorFactory()
}
