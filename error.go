package fox

import "github.com/surge-go/fox/core/errors"

// IErrors 定义 HTTP server 常用错误的工厂接口。
//
// 实现方可以基于业务自己的错误码体系或国际化消息返回 *errors.Error。
// Engine 会通过该接口统一生成请求绑定、鉴权、限流和服务端错误等标准响应。
type IErrors interface {
	// ErrServer 返回服务器内部错误，通常对应 HTTP 500。
	ErrServer() *errors.Error
	// ErrBadRequest 返回错误请求，通常对应 HTTP 400。
	ErrBadRequest() *errors.Error
	// ErrUnauthorized 返回未认证错误，通常对应 HTTP 401。
	ErrUnauthorized() *errors.Error
	// ErrForbidden 返回无权限访问错误，通常对应 HTTP 403。
	ErrForbidden() *errors.Error
	// ErrNotFound 返回资源不存在错误，通常对应 HTTP 404。
	ErrNotFound() *errors.Error
	// ErrConflict 返回资源冲突错误，通常对应 HTTP 409。
	ErrConflict() *errors.Error
	// ErrTooManyRequests 返回请求过于频繁错误，通常对应 HTTP 429。
	ErrTooManyRequests() *errors.Error
	// ErrInvalidParams 返回参数无效错误，通常对应 HTTP 400 或业务参数错误码。
	ErrInvalidParams() *errors.Error
	// ErrRequestTimeout 返回请求处理超时错误，通常对应 HTTP 408。
	ErrRequestTimeout() *errors.Error
	// ErrServiceUnavailable 返回服务暂不可用错误，通常对应 HTTP 503。
	ErrServiceUnavailable() *errors.Error
	// ErrGatewayTimeout 返回网关或下游服务超时错误，通常对应 HTTP 504。
	ErrGatewayTimeout() *errors.Error
}
