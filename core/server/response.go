package server

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
	TraceID string `json:"trace_id,omitempty"`
}

func NewResponse(code int, data any, message string, traceID string) *Response {
	return &Response{
		Code:    code,
		Message: message,
		Data:    data,
		TraceID: traceID,
	}
}
