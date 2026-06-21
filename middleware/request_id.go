package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/surge-go/fox"
)

const defaultRequestIDHeader = "X-Request-ID"

var fallbackRequestIDCounter uint64

// RequestIDGenerator 生成 request id。
type RequestIDGenerator func(*fox.Context) string

// RequestIDConfig 表示 request id 中间件配置。
type RequestIDConfig struct {
	// Header 表示读取和写入 request id 的 HTTP 头，空值使用 X-Request-ID。
	Header string
	// Generator 在请求头和 trace id 都为空时生成 request id，nil 使用默认随机生成器。
	Generator RequestIDGenerator
	// IgnoreHeader 控制是否忽略客户端传入的 Header。
	IgnoreHeader bool
}

// DefaultRequestIDConfig 返回 request id 中间件默认配置。
func DefaultRequestIDConfig() RequestIDConfig {
	return RequestIDConfig{
		Header:    defaultRequestIDHeader,
		Generator: DefaultRequestIDGenerator,
	}
}

// RequestID 返回使用默认配置的 request id 中间件。
func RequestID() fox.HandlerFunc {
	return RequestIDWithConfig(DefaultRequestIDConfig())
}

// RequestIDWithConfig 返回使用自定义配置的 request id 中间件。
func RequestIDWithConfig(cfg RequestIDConfig) fox.HandlerFunc {
	cfg = normalizeRequestIDConfig(cfg)
	return func(c *fox.Context) {
		requestID := ""
		if !cfg.IgnoreHeader {
			requestID = strings.TrimSpace(c.GetHeader(cfg.Header))
		}
		if requestID == "" {
			requestID = strings.TrimSpace(c.TraceID())
		}
		if requestID == "" {
			requestID = strings.TrimSpace(cfg.Generator(c))
		}
		if requestID != "" {
			c.SetRequestID(requestID)
			c.SetHeader(cfg.Header, requestID)
		}
		c.Next()
	}
}

// DefaultRequestIDGenerator 生成 16 字节随机 request id。
func DefaultRequestIDGenerator(*fox.Context) string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return hex.EncodeToString(buf[:])
	}
	seq := atomic.AddUint64(&fallbackRequestIDCounter, 1)
	return strconv.FormatInt(time.Now().UnixNano(), 36) + strconv.FormatUint(seq, 36)
}

func normalizeRequestIDConfig(cfg RequestIDConfig) RequestIDConfig {
	defaults := DefaultRequestIDConfig()
	cfg.Header = strings.TrimSpace(cfg.Header)
	if cfg.Header == "" {
		cfg.Header = defaults.Header
	}
	if cfg.Generator == nil {
		cfg.Generator = defaults.Generator
	}
	return cfg
}
