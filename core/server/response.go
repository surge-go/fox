package server

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func NewResponse(code int, data any, message string) *Response {
	return &Response{
		Code:    code,
		Message: message,
		Data:    data,
	}
}
