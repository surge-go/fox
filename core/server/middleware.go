package server

import (
	"fmt"
	"net"
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
	clientIP := logClientIP(c.ClientIP())
	method := c.RawRequest().Method
	path := c.RawRequest().URL.Path

	fmt.Printf("%s[FOX]%s %s | %s%d%s | %s | %s | %s \"%s\"%s\n",
		colorCyan,
		colorReset,
		start.Format("2006/01/02 - 15:04:05"),
		statusColor(statusCode),
		statusCode,
		colorReset,
		formatLogLatency(latency),
		clientIP,
		method,
		path,
		traceLogFields(c),
	)
}

func logClientIP(ip string) string {
	ip = strings.TrimSpace(ip)
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	}
	if parsed.IsLoopback() {
		if parsed.To4() == nil {
			return "127.0.0.1"
		}
	}
	return ip
}

func statusColor(statusCode int) string {
	switch {
	case statusCode >= http.StatusInternalServerError:
		return colorRed
	case statusCode >= http.StatusBadRequest:
		return colorYellow
	case statusCode >= http.StatusMultipleChoices:
		return colorCyan
	default:
		return colorGreen
	}
}

func traceLogFields(c *Context) string {
	traceID := strings.TrimSpace(c.TraceID())
	spanID := strings.TrimSpace(c.SpanID())
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
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}
