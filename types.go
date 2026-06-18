package fox

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/surge-go/fox/core/errors"
)

const (
	// TraceIDKey 是 Context 中保存 trace id 的键。
	TraceIDKey = "trace_id"
	// SpanIDKey 是 Context 中保存 span id 的键。
	SpanIDKey = "span_id"
)

// Response 表示统一 JSON 响应体。
type Response struct {
	// Code 表示业务状态码，默认实现中通常与 HTTP 状态码一致。
	Code int `json:"code"`
	// Data 表示响应数据；错误响应通常为 nil。
	Data any `json:"data"`
	// Message 表示用户可见的响应消息。
	Message string `json:"message"`
	// TraceID 表示当前请求的链路追踪 ID。
	TraceID string `json:"trace_id,omitempty"`
}

// WithTraceID 设置响应中的链路追踪 ID，并返回当前响应对象。
func (r *Response) WithTraceID(traceID string) *Response {
	r.TraceID = traceID
	return r
}

// NewResponse 创建统一 JSON 响应体。
//
// traceID 为可选参数；传入时会写入 Response.TraceID。
func NewResponse(code int, data any, message string, traceID ...string) *Response {
	response := Response{
		Code:    code,
		Data:    data,
		Message: message,
	}
	if traceID != nil && len(traceID) > 0 {
		response.TraceID = traceID[0]
	}
	return &response
}

// HandlerFunc 表示 server 的请求处理函数。
type HandlerFunc func(*Context)

// HandlersChain 表示按顺序执行的处理函数链。
type HandlersChain []HandlerFunc

// Logger 表示 fox 生态中请求日志中间件可使用的日志接口。
type Logger interface {
	Printf(format string, args ...any)
}

// toGinHandler 将 HandlerFunc 转换为 gin.HandlerFunc。
func toGinHandler(h HandlerFunc, errors IErrors) gin.HandlerFunc {
	errors = normalizeEngineErrorFactory(errors)
	return func(gc *gin.Context) {
		h(&Context{ctx: gc, errors: errors})
	}
}

// toGinHandlers 将 HandlersChain 转换为 []gin.HandlerFunc。
func toGinHandlers(handlers HandlersChain, errors IErrors) []gin.HandlerFunc {
	ginHandlers := make([]gin.HandlerFunc, len(handlers))
	for i, h := range handlers {
		ginHandlers[i] = toGinHandler(h, errors)
	}
	return ginHandlers
}

func newEngineErrorFactory(errors ...IErrors) IErrors {
	if len(errors) > 0 && errors[0] != nil {
		return errors[0]
	}
	return defaultErrorFactory()
}

func normalizeEngineErrorFactory(errors IErrors) IErrors {
	if errors != nil {
		return errors
	}
	return defaultErrorFactory()
}

func defaultErrorFactory() IErrors {
	return &Err{}
}

// Err 是默认的 HTTP 错误工厂实现。
//
// 默认实现使用 HTTP 状态码作为业务错误码，并返回英文短消息。
type Err struct{}

// ErrServer 返回服务器内部错误，通常对应 HTTP 500。
func (e *Err) ErrServer() *errors.Error {
	return errors.NewWithStatus(http.StatusInternalServerError, http.StatusInternalServerError, "internal server error")
}

// ErrBadRequest 返回错误请求，通常对应 HTTP 400。
func (e *Err) ErrBadRequest() *errors.Error {
	return errors.NewWithStatus(http.StatusBadRequest, http.StatusBadRequest, "bad request")
}

// ErrUnauthorized 返回未认证错误，通常对应 HTTP 401。
func (e *Err) ErrUnauthorized() *errors.Error {
	return errors.NewWithStatus(http.StatusUnauthorized, http.StatusUnauthorized, "unauthorized")
}

// ErrForbidden 返回无权限访问错误，通常对应 HTTP 403。
func (e *Err) ErrForbidden() *errors.Error {
	return errors.NewWithStatus(http.StatusForbidden, http.StatusForbidden, "forbidden")
}

// ErrNotFound 返回资源不存在错误，通常对应 HTTP 404。
func (e *Err) ErrNotFound() *errors.Error {
	return errors.NewWithStatus(http.StatusNotFound, http.StatusNotFound, "not found")
}

// ErrConflict 返回资源冲突错误，通常对应 HTTP 409。
func (e *Err) ErrConflict() *errors.Error {
	return errors.NewWithStatus(http.StatusConflict, http.StatusConflict, "conflict")
}

// ErrTooManyRequests 返回请求过于频繁错误，通常对应 HTTP 429。
func (e *Err) ErrTooManyRequests() *errors.Error {
	return errors.NewWithStatus(http.StatusTooManyRequests, http.StatusTooManyRequests, "too many requests")
}

// ErrInvalidParams 返回参数无效错误，通常对应 HTTP 400。
func (e *Err) ErrInvalidParams() *errors.Error {
	return errors.NewWithStatus(http.StatusBadRequest, http.StatusBadRequest, "invalid params")
}

// ErrRequestTimeout 返回请求处理超时错误，通常对应 HTTP 408。
func (e *Err) ErrRequestTimeout() *errors.Error {
	return errors.NewWithStatus(http.StatusRequestTimeout, http.StatusRequestTimeout, "request timeout")
}

// ErrServiceUnavailable 返回服务暂不可用错误，通常对应 HTTP 503。
func (e *Err) ErrServiceUnavailable() *errors.Error {
	return errors.NewWithStatus(http.StatusServiceUnavailable, http.StatusServiceUnavailable, "service unavailable")
}

// ErrGatewayTimeout 返回网关或下游服务超时错误，通常对应 HTTP 504。
func (e *Err) ErrGatewayTimeout() *errors.Error {
	return errors.NewWithStatus(http.StatusGatewayTimeout, http.StatusGatewayTimeout, "gateway timeout")
}
