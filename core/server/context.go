package server

import (
	"context"
	"io/fs"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/surge-go/fox/core/errors"
)

// Context 封装 gin.Context，对外提供类型安全的请求处理接口。
// 同时实现了 context.Context 接口，可直接作为标准上下文传递。
type Context struct {
	ctx *gin.Context
}

// ===== 底层访问 =====

// StdContext 返回标准库 context.Context，用于传递到需要标准上下文的函数。
func (c *Context) StdContext() context.Context {
	return c.ctx.Request.Context()
}

// RawRequest 返回原始 *http.Request，用于需要直接操作标准库请求对象的场景。
func (c *Context) RawRequest() *http.Request {
	return c.ctx.Request
}

// WithContext 替换底层请求的标准 context.Context。
//
// 适用于 tracing、认证、超时控制等需要把派生 context 传递给后续 Handler
// 以及数据库、Redis、HTTP client 等下游库的场景。
func (c *Context) WithContext(ctx context.Context) {
	if ctx == nil {
		return
	}
	c.ctx.Request = c.ctx.Request.WithContext(ctx)
}

// Status 返回响应的 HTTP 状态码。
func (c *Context) Status() int {
	return c.ctx.Writer.Status()
}

// WithValue 向请求上下文中写入键值对，会替换底层的 *http.Request。
func (c *Context) WithValue(key, value any) {
	c.ctx.Request = c.ctx.Request.WithContext(context.WithValue(c.ctx.Request.Context(), key, value))
}

// Deadline 返回请求上下文的截止时间和是否设置。
func (c *Context) Deadline() (deadline time.Time, ok bool) {
	return c.StdContext().Deadline()
}

// Done 返回请求上下文的取消信号通道。
func (c *Context) Done() <-chan struct{} {
	return c.StdContext().Done()
}

// Err 返回请求上下文的取消原因。
func (c *Context) Err() error {
	return c.StdContext().Err()
}

// Value 从请求上下文中获取指定键的值。
func (c *Context) Value(key any) any {
	return c.StdContext().Value(key)
}

// ===== 链路追踪 =====

// SetTraceID 在当前请求上下文中保存 trace id。
//
// trace id 通常由 tracing 中间件从 OpenTelemetry span context 中写入，
// 用于访问日志、错误排查和跨服务链路关联。业务代码一般不需要手动设置；
// 只有在接入外部链路追踪系统或自定义 tracing 中间件时才建议调用。
func (c *Context) SetTraceID(id string) {
	c.Set(TraceIDKey, id)
}

// TraceID 返回当前请求上下文中的 trace id。
//
// 未注册 tracing 中间件、请求被 tracing SkipFunc 跳过，或尚未写入 trace id 时
// 返回空字符串。
func (c *Context) TraceID() string {
	return c.GetString(TraceIDKey)
}

// SetSpanID 在当前请求上下文中保存 span id。
//
// span id 表示当前 HTTP server span 的标识，通常由 tracing 中间件自动写入。
// 业务代码一般不需要手动设置。
func (c *Context) SetSpanID(id string) {
	c.Set(SpanIDKey, id)
}

// SpanID 返回当前请求上下文中的 span id。
//
// 未注册 tracing 中间件、请求被 tracing SkipFunc 跳过，或尚未写入 span id 时
// 返回空字符串。
func (c *Context) SpanID() string {
	return c.GetString(SpanIDKey)
}

// ===== 上下文生命周期 =====

// Copy 创建上下文的副本，用于在 goroutine 中安全使用。
func (c *Context) Copy() *Context {
	return &Context{c.ctx.Copy()}
}

// Next 跳过当前 Handler，执行链中下一个 Handler。
func (c *Context) Next() {
	c.ctx.Next()
}

// FullPath 返回匹配到的路由模板路径（如 "/user/:id"），未匹配时返回空字符串。
func (c *Context) FullPath() string {
	return c.ctx.FullPath()
}

// ===== 请求中止与错误处理 =====

// Abort 中止当前请求，后续 Handler 不再执行。
func (c *Context) Abort() {
	c.ctx.Abort()
}

// IsAborted 判断当前请求是否已被中止。
func (c *Context) IsAborted() bool {
	return c.ctx.IsAborted()
}

// AbortWithStatus 中止请求并写入 HTTP 状态码。
func (c *Context) AbortWithStatus(status int) {
	c.ctx.AbortWithStatus(status)
}

// AbortWithStatusJSON 中止请求并写入 HTTP 状态码和 JSON 响应体。
func (c *Context) AbortWithStatusJSON(status int, data any) {
	c.ctx.AbortWithStatusJSON(status, data)
}

// ===== 键值存储与数据共享 =====

// Set 在上下文中设置键值对，可在 Handler 链中传递数据。
func (c *Context) Set(key string, value any) {
	c.ctx.Set(key, value)
}

// Get 从上下文中获取键值对。
// 返回 (value, true) 表示存在；返回 (nil, false) 表示不存在。
func (c *Context) Get(key string) (val any, ok bool) {
	return c.ctx.Get(key)
}

// GetString 从上下文中获取 string 类型的值，不存在时返回 ""。
func (c *Context) GetString(key string) string {
	return c.ctx.GetString(key)
}

// GetBool 从上下文中获取 bool 类型的值，不存在时返回 false。
func (c *Context) GetBool(key string) bool {
	return c.ctx.GetBool(key)
}

// GetInt 从上下文中获取 int 类型的值，不存在时返回 0。
func (c *Context) GetInt(key string) int {
	return c.ctx.GetInt(key)
}

// GetInt64 从上下文中获取 int64 类型的值，不存在时返回 0。
func (c *Context) GetInt64(key string) int64 {
	return c.ctx.GetInt64(key)
}

// GetUint 从上下文中获取 uint 类型的值，不存在时返回 0。
func (c *Context) GetUint(key string) uint {
	return c.ctx.GetUint(key)
}

// GetUint64 从上下文中获取 uint64 类型的值，不存在时返回 0。
func (c *Context) GetUint64(key string) uint64 {
	return c.ctx.GetUint64(key)
}

// GetFloat64 从上下文中获取 float64 类型的值，不存在时返回 0。
func (c *Context) GetFloat64(key string) float64 {
	return c.ctx.GetFloat64(key)
}

// GetTime 从上下文中获取 time.Time 类型的值，不存在时返回零值。
func (c *Context) GetTime(key string) time.Time {
	return c.ctx.GetTime(key)
}

// GetDuration 从上下文中获取 time.Duration 类型的值，不存在时返回 0。
func (c *Context) GetDuration(key string) time.Duration {
	return c.ctx.GetDuration(key)
}

// GetStringSlice 从上下文中获取 []string 类型的值，不存在时返回 nil。
func (c *Context) GetStringSlice(key string) []string {
	return c.ctx.GetStringSlice(key)
}

// GetStringMap 从上下文中获取 map[string]any 类型的值，不存在时返回 nil。
func (c *Context) GetStringMap(key string) map[string]any {
	return c.ctx.GetStringMap(key)
}

// GetStringMapString 从上下文中获取 map[string]string 类型的值，不存在时返回 nil。
func (c *Context) GetStringMapString(key string) map[string]string {
	return c.ctx.GetStringMapString(key)
}

// GetStringMapStringSlice 从上下文中获取 map[string][]string 类型的值，不存在时返回 nil。
func (c *Context) GetStringMapStringSlice(key string) map[string][]string {
	return c.ctx.GetStringMapStringSlice(key)
}

// MustGet 从上下文中获取值，不存在时 panic。
func (c *Context) MustGet(key string) any {
	return c.ctx.MustGet(key)
}

// Delete 从上下文中删除指定键。
func (c *Context) Delete(key string) {
	c.ctx.Delete(key)
}

// ===== 请求绑定 =====

// Bind 根据 Content-Type 自动绑定请求参数到结构体，校验失败时返回 400 响应。
func (c *Context) Bind(obj any) error {
	return c.bindOrFail(c.ctx.ShouldBind(obj))
}

// BindJSON 绑定 JSON 请求体到结构体，校验失败时返回 400 响应。
func (c *Context) BindJSON(obj any) error {
	return c.bindOrFail(c.ctx.ShouldBindJSON(obj))
}

// BindQuery 绑定 URL 查询参数到结构体，校验失败时返回 400 响应。
func (c *Context) BindQuery(obj any) error {
	return c.bindOrFail(c.ctx.ShouldBindQuery(obj))
}

// BindURI 绑定 URI 路径参数到结构体，校验失败时返回 400 响应。
func (c *Context) BindURI(obj any) error {
	return c.bindOrFail(c.ctx.ShouldBindUri(obj))
}

// bindOrFail 统一处理绑定错误：失败时写入 400 标准响应体并中止请求。
func (c *Context) bindOrFail(err error) error {
	if err != nil {
		c.JSON(http.StatusBadRequest, c.newResponse(http.StatusBadRequest, nil, err.Error()))
		c.Abort()
	}
	return err
}

// ===== URL 与路径参数 =====

// Param 获取路由路径参数（如 /user/:id 中的 id）。
func (c *Context) Param(key string) string {
	return c.ctx.Param(key)
}

// Query 获取 URL 查询参数（如 ?id=1 中的 id），不存在时返回空字符串。
func (c *Context) Query(key string) string {
	return c.ctx.Query(key)
}

// DefaultQuery 获取 URL 查询参数，不存在时返回指定的默认值。
func (c *Context) DefaultQuery(key, defaultValue string) string {
	return c.ctx.DefaultQuery(key, defaultValue)
}

// GetQuery 获取 URL 查询参数，返回值和是否存在。
func (c *Context) GetQuery(key string) (string, bool) {
	return c.ctx.GetQuery(key)
}

// QueryArray 获取 URL 查询参数的多个值（如 ?id=1&id=2 → ["1", "2"]）。
func (c *Context) QueryArray(key string) []string {
	return c.ctx.QueryArray(key)
}

// QueryMap 获取 URL 查询参数中的 map 结构（如 ?user[name]=tom → {"name": "tom"}）。
func (c *Context) QueryMap(key string) map[string]string {
	return c.ctx.QueryMap(key)
}

// ===== 表单数据 =====

// PostForm 获取 POST 表单参数，不存在时返回空字符串。
func (c *Context) PostForm(key string) string {
	return c.ctx.PostForm(key)
}

// DefaultPostForm 获取 POST 表单参数，不存在时返回指定的默认值。
func (c *Context) DefaultPostForm(key, defaultValue string) string {
	return c.ctx.DefaultPostForm(key, defaultValue)
}

// GetPostForm 获取 POST 表单参数，返回值和是否存在。
func (c *Context) GetPostForm(key string) (string, bool) {
	return c.ctx.GetPostForm(key)
}

// PostFormArray 获取 POST 表单参数的多个值。
func (c *Context) PostFormArray(key string) []string {
	return c.ctx.PostFormArray(key)
}

// PostFormMap 获取 POST 表单参数中的 map 结构。
func (c *Context) PostFormMap(key string) map[string]string {
	return c.ctx.PostFormMap(key)
}

// ===== 文件上传 =====

// FormFile 获取上传的文件头信息。
func (c *Context) FormFile(key string) (*multipart.FileHeader, error) {
	return c.ctx.FormFile(key)
}

// MultipartForm 获取完整的 multipart 表单数据。
func (c *Context) MultipartForm() (*multipart.Form, error) {
	return c.ctx.MultipartForm()
}

// SaveUploadedFile 将上传的文件保存到指定路径，可通过 perm 指定文件权限。
func (c *Context) SaveUploadedFile(file *multipart.FileHeader, dst string, perm ...fs.FileMode) error {
	return c.ctx.SaveUploadedFile(file, dst, perm...)
}

// ===== 请求元信息 =====

// ClientIP 获取客户端 IP 地址，受 TrustedProxies 配置影响。
func (c *Context) ClientIP() string {
	return c.ctx.ClientIP()
}

// ContentType 获取请求体的 Content-Type。
func (c *Context) ContentType() string {
	return c.ctx.ContentType()
}

// GetHeader 获取请求头的值，键名大小写不敏感。
func (c *Context) GetHeader(key string) string {
	return c.ctx.GetHeader(key)
}

// GetCookie 获取请求中指定名称的 Cookie，不存在时返回错误。
func (c *Context) GetCookie(name string) (string, error) {
	return c.ctx.Cookie(name)
}

// ===== 响应写入 =====

// SetHeader 设置响应头键值对。
func (c *Context) SetHeader(key, value string) {
	c.ctx.Header(key, value)
}

// GetResponseHeader 获取响应头的值，键名大小写不敏感。
func (c *Context) GetResponseHeader(key string) string {
	return c.ctx.Writer.Header().Get(key)
}

// SetCookie 设置响应 Cookie。
// secure 控制是否仅通过 HTTPS 传输；httpOnly 控制是否禁止 JS 访问。
func (c *Context) SetCookie(name, value string, maxAge int, path, domain string, secure, httpOnly bool) {
	c.ctx.SetCookie(name, value, maxAge, path, domain, secure, httpOnly)
}

// JSON 写入 HTTP 状态码并序列化 data 为 JSON 响应体，Content-Type 为 application/json。
func (c *Context) JSON(status int, data any) {
	c.ctx.JSON(status, data)
}

// HTML 渲染指定模板名为 HTML 响应，data 为模板变量。
func (c *Context) HTML(status int, name string, data any) {
	c.ctx.HTML(status, name, data)
}

// String 写入 HTTP 状态码和纯文本响应，Content-Type 为 text/plain。
func (c *Context) String(status int, text string) {
	c.ctx.String(status, text)
}

// File 以流式方式返回指定路径的文件作为响应。
func (c *Context) File(path string) {
	c.ctx.File(path)
}

// Redirect 发起 HTTP 重定向，status 应为 3xx 状态码（如 301、302、307）。
func (c *Context) Redirect(status int, path string) {
	c.ctx.Redirect(status, path)
}

// ===== 高级响应 =====

// Ok 返回 200 成功响应，序列化 data 为标准 Response 体后中止请求。
func (c *Context) Ok(data any) {
	c.JSON(http.StatusOK, c.newResponse(200, data, "success"))
	c.Abort()
}

// Fail 返回错误响应。
// 传入 nil 时等同于 Ok(nil)。
// 传入非 *errors.Error 时返回 500 并隐藏内部细节；
// 传入 *errors.Error 时使用其 Status 作为 HTTP 状态码，Code 作为业务码。
func (c *Context) Fail(err error) {
	if err == nil {
		c.Ok(nil)
		return
	}
	if e, ok := errors.As(err); ok {
		c.JSON(validHTTPStatusOrDefault(e.Status()), c.newResponse(e.Code, nil, e.Message))
		c.Abort()
		return
	}

	c.JSON(http.StatusInternalServerError, c.newResponse(http.StatusInternalServerError, nil, "internal server error"))
	c.Abort()
}

func (c *Context) newResponse(code int, data any, message string) *Response {
	resp := NewResponse(code, data, message)
	resp.SetTraceID(c.TraceID())
	return resp
}

func validHTTPStatusOrDefault(status int) int {
	if status < 100 || status > 599 {
		return http.StatusInternalServerError
	}
	return status
}
