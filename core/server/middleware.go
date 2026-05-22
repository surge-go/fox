package server

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

const (
	// TraceIDKey 是 server.Context 中保存 OpenTelemetry trace id 的键。
	TraceIDKey = "trace_id"
	// SpanIDKey 是 server.Context 中保存 OpenTelemetry span id 的键。
	SpanIDKey = "span_id"
)

// recoveryMiddleware 内置的 Recovery 中间件。
func recoveryMiddleware(mode Mode) HandlerFunc {
	return func(c *Context) {
		defer func() {
			if err := recover(); err != nil {
				if mode == ModeRelease {
					fmt.Println("[Recovery] panic recovered")
				} else {
					stack := debug.Stack()
					fmt.Printf("[Recovery] panic recovered:\n%v\n%s\n", err, stack)
				}

				c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
					"code":    500,
					"message": "Internal Server Error",
				})
			}
		}()

		c.Next()
	}
}

// loggerMiddleware 内置日志中间件，记录每个请求的基本信息。
func loggerMiddleware() HandlerFunc {
	return func(c *Context) {
		start := time.Now()

		defer func() {
			statusCode := c.Status()
			if recovered := recover(); recovered != nil {
				statusCode = http.StatusInternalServerError
				logRequest(c, start, statusCode)
				panic(recovered)
			}
			logRequest(c, start, statusCode)
		}()

		c.Next()
	}
}

func logRequest(c *Context, start time.Time, statusCode int) {
	latency := time.Since(start)
	clientIP := c.ClientIP()
	method := c.RawRequest().Method
	path := c.RawRequest().URL.Path

	fmt.Printf("[FOX] %s | %3d | %10s | %15s | %-7s \"%s\"%s\n",
		start.Format("2006/01/02 - 15:04:05"),
		statusCode,
		formatLogLatency(latency),
		clientIP,
		method,
		path,
		traceLogFields(c),
	)
}

func traceLogFields(c *Context) string {
	traceID := strings.TrimSpace(c.GetString(TraceIDKey))
	spanID := strings.TrimSpace(c.GetString(SpanIDKey))
	if traceID == "" && spanID == "" {
		return ""
	}

	if traceID == "" {
		return fmt.Sprintf(" | span_id=%s", spanID)
	}
	if spanID == "" {
		return fmt.Sprintf(" | trace_id=%s", traceID)
	}
	return fmt.Sprintf(" | trace_id=%s | span_id=%s", traceID, spanID)
}

func formatLogLatency(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%7dns", d.Nanoseconds())
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%7dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%7dms", d.Milliseconds())
	}
	return fmt.Sprintf("%7.2fs", d.Seconds())
}
