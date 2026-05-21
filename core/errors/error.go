package errors

import (
	"errors"
	"fmt"
	"net/http"
)

const defaultStatus = http.StatusInternalServerError

// Error 框架错误类型
type Error struct {
	Code    int    `json:"code"`    // 业务错误码
	Message string `json:"message"` // 用户可见消息
	err     error  // 原始错误
	status  int    // HTTP 状态码
}

// New 创建错误
func New(code int, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
		status:  defaultStatus,
	}
}

// NewWithStatus 创建带 HTTP 状态码的错误
func NewWithStatus(code int, status int, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
		status:  status,
	}
}

// clone 复制一份，避免链式调用污染预定义错误
func (e *Error) clone() *Error {
	if e == nil {
		return &Error{status: defaultStatus}
	}
	return &Error{
		Code:    e.Code,
		Message: e.Message,
		err:     e.err,
		status:  e.status,
	}
}

// WithErr 附加原始错误（返回新实例）
func (e *Error) WithErr(err error) *Error {
	n := e.clone()
	n.err = err
	return n
}

// WithStatus 设置 HTTP 状态码（返回新实例）
func (e *Error) WithStatus(status int) *Error {
	n := e.clone()
	n.status = status
	return n
}

// WithMessage 覆盖消息（返回新实例）
func (e *Error) WithMessage(message string) *Error {
	n := e.clone()
	n.Message = message
	return n
}

// WithMessagef 格式化覆盖消息（返回新实例）
func (e *Error) WithMessagef(format string, args ...any) *Error {
	n := e.clone()
	n.Message = fmt.Sprintf(format, args...)
	return n
}

func (e *Error) Is(target error) bool {
	if e == nil || target == nil {
		return false
	}
	var t *Error
	ok := errors.As(target, &t)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// ===== 访问器 =====

// Error 实现 error 接口
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.err)
	}
	return e.Message
}

// Status 获取 HTTP 状态码
func (e *Error) Status() int {
	if e == nil || e.status == 0 {
		return defaultStatus
	}
	return e.status
}

// Unwrap 实现标准库 errors.Unwrap 接口
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// ===== 辅助函数 =====

// Wrap 包装已有错误为 fox Error
func Wrap(err error, code int, message string) *Error {
	if err == nil {
		return New(code, message)
	}
	return &Error{
		Code:    code,
		Message: message,
		err:     err,
		status:  defaultStatus,
	}
}

// WrapWithStatus 包装已有错误（带 HTTP 状态码）
func WrapWithStatus(err error, code int, status int, message string) *Error {
	if err == nil {
		return NewWithStatus(code, status, message)
	}
	return &Error{
		Code:    code,
		Message: message,
		err:     err,
		status:  status,
	}
}

// GetCode 从 error 中提取业务错误码，非 fox Error 返回 -1
func GetCode(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return -1
}

// GetStatus 从 error 中提取 HTTP 状态码，非 fox Error 返回 500
func GetStatus(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Status()
	}
	return http.StatusInternalServerError
}

// IsCode 判断 error 是否匹配指定业务错误码
func IsCode(err error, code int) bool {
	return GetCode(err) == code
}

// As 标准库 errors.As 的快捷方式
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

// Is 标准库 errors.Is 的透传
func Is(err, target error) bool {
	return errors.Is(err, target)
}
