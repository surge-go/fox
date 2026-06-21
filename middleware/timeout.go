package middleware

import (
	"context"
	"time"

	"github.com/surge-go/fox"
)

const defaultTimeoutDuration = 30 * time.Second

// TimeoutErrorHandler 处理请求超时响应。
type TimeoutErrorHandler func(*fox.Context)

// TimeoutConfig 表示请求超时中间件配置。
type TimeoutConfig struct {
	// Duration 表示请求处理超时时间，0 使用默认值，负数表示不启用超时。
	Duration time.Duration
	// ErrorHandler 处理超时响应，nil 使用默认 408 JSON 响应。
	ErrorHandler TimeoutErrorHandler
}

// DefaultTimeoutConfig 返回请求超时中间件默认配置。
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		Duration:     defaultTimeoutDuration,
		ErrorHandler: defaultTimeoutErrorHandler,
	}
}

// Timeout 返回使用默认配置的请求超时中间件。
func Timeout() fox.HandlerFunc {
	return TimeoutWithConfig(DefaultTimeoutConfig())
}

// TimeoutWithConfig 返回使用自定义配置的请求超时中间件。
//
// 该中间件通过标准 context.Context 传播 deadline。handler 或 service 应该监听
// c.StdContext().Done() 并及时返回；如果处理链返回时已经超时且响应尚未写入，
// 中间件会写入超时响应。
func TimeoutWithConfig(cfg TimeoutConfig) fox.HandlerFunc {
	cfg = normalizeTimeoutConfig(cfg)
	return func(c *fox.Context) {
		if cfg.Duration < 0 {
			c.Next()
			return
		}

		ctx, cancel := context.WithTimeout(c.StdContext(), cfg.Duration)
		defer cancel()
		c.WithContext(ctx)

		c.Next()

		if ctx.Err() == context.DeadlineExceeded && !c.Written() {
			cfg.ErrorHandler(c)
		}
	}
}

func normalizeTimeoutConfig(cfg TimeoutConfig) TimeoutConfig {
	defaults := DefaultTimeoutConfig()
	if cfg.Duration == 0 {
		cfg.Duration = defaults.Duration
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = defaults.ErrorHandler
	}
	return cfg
}

func defaultTimeoutErrorHandler(c *fox.Context) {
	c.Fail(c.Errors().ErrRequestTimeout())
}
