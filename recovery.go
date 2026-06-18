package fox

import (
	"fmt"
	"runtime/debug"
)

// recoveryMiddleware 捕获 handler panic，避免单个请求异常逃逸到 net/http。
func recoveryMiddleware(mode Mode) HandlerFunc {
	return func(c *Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				if mode == ModeRelease {
					fmt.Println("[Recovery] panic recovered")
				} else {
					fmt.Printf("[Recovery] panic recovered:\n%v\n%s\n", recovered, debug.Stack())
				}
				if c.Written() {
					c.Abort()
					return
				}
				c.Fail(c.errorFactory().ErrServer())
			}
		}()
		c.Next()
	}
}
