package middleware

import (
	"runtime/debug"

	"github.com/surge-go/fox"
)

// Recovery 捕获 handler panic，避免单个请求异常逃逸到 net/http。
func Recovery() fox.HandlerFunc {
	return RecoveryWithConfig(RecoveryConfig{})
}

// RecoveryConfig 表示 recovery 中间件配置。
type RecoveryConfig struct {
	// Logger 用于输出 panic 日志，nil 时使用标准输出。
	Logger fox.Logger
	// EnableStack 控制是否输出 panic 堆栈。
	EnableStack bool
}

// RecoveryWithConfig 返回使用自定义配置的 recovery 中间件。
func RecoveryWithConfig(cfg RecoveryConfig) fox.HandlerFunc {
	log := cfg.Logger
	if log == nil {
		log = defaultLogger{}
	}
	return func(c *fox.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				if cfg.EnableStack {
					log.Printf("[Recovery] panic recovered:\n%v\n%s", recovered, debug.Stack())
				} else {
					log.Printf("[Recovery] panic recovered: %v", recovered)
				}
				if c.Written() {
					c.Abort()
					return
				}
				c.Fail(c.Errors().ErrServer())
			}
		}()
		c.Next()
	}
}
