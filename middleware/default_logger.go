package middleware

import "fmt"

type defaultLogger struct{}

func (defaultLogger) Printf(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}
