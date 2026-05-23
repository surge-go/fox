package server

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
	// TraceID 是当前请求的链路追踪 ID，未启用 tracing 或未写入时不输出。
	TraceID string `json:"trace_id,omitempty"`
}

func NewResponse(code int, data any, message string) *Response {
	return &Response{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

// SetTraceID 设置响应体中的链路追踪 ID。
func (r *Response) SetTraceID(traceID string) {
	if r == nil {
		return
	}
	r.TraceID = traceID
}
