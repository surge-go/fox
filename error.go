package fox

import "github.com/surge-go/fox/core/errors"

// IErrors 定义 HTTP server 常用错误的工厂接口。
//
// 实现方可以基于业务自己的错误码体系或国际化消息返回 *errors.Error。
// Engine 会通过该接口统一生成请求绑定、鉴权、限流和服务端错误等标准响应。
type IErrors interface {
	// ErrServer 返回服务器内部错误，通常对应 HTTP 500。
	ErrServer() *errors.Error
	// ErrTooManyRequests 返回请求过于频繁错误，通常对应 HTTP 429。
	ErrTooManyRequests() *errors.Error
	// ErrPayloadTooLarge 返回请求体过大错误，通常对应 HTTP 413。
	ErrPayloadTooLarge() *errors.Error
	// ErrInvalidParams 返回参数无效错误，通常对应 HTTP 400 或业务参数错误码。
	ErrInvalidParams() *errors.Error
	// ErrRequestTimeout 返回请求处理超时错误，通常对应 HTTP 408。
	ErrRequestTimeout() *errors.Error
	// ErrServiceUnavailable 返回服务暂不可用错误，通常对应 HTTP 503。
	ErrServiceUnavailable() *errors.Error
}
