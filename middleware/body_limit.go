package middleware

import (
	"net/http"

	"github.com/surge-go/fox"
)

const defaultBodyLimitMaxBytes int64 = 10 << 20

// BodyLimitErrorHandler 处理请求体超过限制的请求。
type BodyLimitErrorHandler func(*fox.Context, int64)

// BodyLimitConfig 表示请求体大小限制中间件配置。
type BodyLimitConfig struct {
	// MaxBytes 表示允许读取的最大请求体字节数，0 使用默认值，负数表示不限制。
	MaxBytes int64
	// ErrorHandler 处理超过限制的请求，nil 使用默认 413 JSON 响应。
	ErrorHandler BodyLimitErrorHandler
}

// DefaultBodyLimitConfig 返回请求体大小限制中间件默认配置。
func DefaultBodyLimitConfig() BodyLimitConfig {
	return BodyLimitConfig{
		MaxBytes:     defaultBodyLimitMaxBytes,
		ErrorHandler: defaultBodyLimitErrorHandler,
	}
}

// BodyLimit 返回使用默认配置的请求体大小限制中间件。
func BodyLimit() fox.HandlerFunc {
	return BodyLimitWithConfig(DefaultBodyLimitConfig())
}

// BodyLimitWithConfig 返回使用自定义配置的请求体大小限制中间件。
func BodyLimitWithConfig(cfg BodyLimitConfig) fox.HandlerFunc {
	cfg = normalizeBodyLimitConfig(cfg)
	return func(c *fox.Context) {
		if cfg.MaxBytes < 0 || c.RawRequest().Body == nil {
			c.Next()
			return
		}
		if c.RawRequest().ContentLength > cfg.MaxBytes {
			cfg.ErrorHandler(c, cfg.MaxBytes)
			return
		}

		c.RawRequest().Body = http.MaxBytesReader(c.RawWriter(), c.RawRequest().Body, cfg.MaxBytes)
		c.Next()
	}
}

func normalizeBodyLimitConfig(cfg BodyLimitConfig) BodyLimitConfig {
	defaults := DefaultBodyLimitConfig()
	if cfg.MaxBytes == 0 {
		cfg.MaxBytes = defaults.MaxBytes
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = defaults.ErrorHandler
	}
	return cfg
}

func defaultBodyLimitErrorHandler(c *fox.Context, _ int64) {
	c.Fail(c.Errors().ErrPayloadTooLarge())
}
