package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/surge-go/fox"
)

const (
	logColorReset   = "\033[0m"
	logColorRed     = "\033[31m"
	logColorYellow  = "\033[33m"
	logColorGreen   = "\033[32m"
	logColorCyan    = "\033[36m"
	logColorMagenta = "\033[35m"
	logColorWhite   = "\033[37m"
)

// LogFields 表示一次请求日志可用的字段。
type LogFields struct {
	Time      time.Time
	Method    string
	Path      string
	Query     string
	Status    int
	ClientIP  string
	Latency   time.Duration
	UserAgent string
	TraceID   string
}

// LogFormatter 将请求字段格式化为一行日志。
type LogFormatter func(LogFields) string

// LoggerConfig 表示请求日志中间件配置。
type LoggerConfig struct {
	// Logger 是日志输出目标，nil 使用标准输出 logger。
	Logger fox.Logger
	// Formatter 自定义日志格式，nil 使用 DefaultLogFormatter。
	Formatter LogFormatter
	// SkipPaths 表示不打印日志的请求路径。
	SkipPaths []string
}

// Logger 返回使用默认配置的请求日志中间件。
func Logger() fox.HandlerFunc {
	return LoggerWithConfig(LoggerConfig{})
}

// LoggerWithConfig 返回使用自定义配置的请求日志中间件。
func LoggerWithConfig(cfg LoggerConfig) fox.HandlerFunc {
	log := cfg.Logger
	if log == nil {
		log = defaultLogger{}
	}
	formatter := cfg.Formatter
	if formatter == nil {
		formatter = DefaultLogFormatter
	}
	skipPaths := make(map[string]struct{}, len(cfg.SkipPaths))
	for _, path := range cfg.SkipPaths {
		path = strings.TrimSpace(path)
		if path != "" {
			skipPaths[path] = struct{}{}
		}
	}

	return func(c *fox.Context) {
		start := time.Now()
		req := c.RawRequest()
		path := req.URL.Path

		c.Next()

		if _, skip := skipPaths[path]; skip {
			return
		}
		fields := LogFields{
			Time:      start,
			Method:    req.Method,
			Path:      path,
			Query:     req.URL.RawQuery,
			Status:    c.Status(),
			ClientIP:  c.ClientIP(),
			Latency:   time.Since(start),
			UserAgent: req.UserAgent(),
			TraceID:   c.TraceID(),
		}
		if fields.Status == 0 {
			fields.Status = http.StatusOK
		}
		log.Printf("%s", strings.TrimRight(formatter(fields), "\n"))
	}
}

// DefaultLogFormatter 返回默认请求日志格式，格式化结果不包含换行。
func DefaultLogFormatter(fields LogFields) string {
	target := fields.Path
	if fields.Query != "" {
		target += "?" + fields.Query
	}
	line := fmt.Sprintf(
		"%s %s | %s | %s | %s | %s %q",
		colorize(logColorCyan, "[FOX]"),
		fields.Time.Format("2006/01/02 - 15:04:05"),
		colorize(statusColor(fields.Status), fmt.Sprintf("%3d", fields.Status)),
		formatLatency(fields.Latency),
		normalizeLoopbackIP(fields.ClientIP),
		fields.Method,
		target,
	)
	if fields.TraceID != "" {
		line += " | " + colorize(logColorWhite, fields.TraceID)
	}
	return line
}

func formatLatency(latency time.Duration) string {
	switch {
	case latency >= time.Second:
		return latency.Truncate(time.Millisecond).String()
	case latency >= time.Millisecond:
		return latency.Truncate(time.Microsecond).String()
	default:
		return latency.Truncate(time.Microsecond).String()
	}
}

func statusColor(status int) string {
	switch {
	case status >= 500:
		return logColorRed
	case status >= 400:
		return logColorYellow
	case status >= 300:
		return logColorCyan
	default:
		return logColorGreen
	}
}

func normalizeLoopbackIP(ip string) string {
	ip = strings.TrimSpace(ip)
	switch ip {
	case "::1", "0:0:0:0:0:0:0:1":
		return "127.0.0.1"
	default:
		return ip
	}
}

func colorize(color, value string) string {
	if value == "" {
		return value
	}
	return color + value + logColorReset
}
