package middleware

import (
	"bytes"
	"fmt"
)

type memoryLogger struct {
	buf bytes.Buffer
}

func (l *memoryLogger) Printf(format string, args ...any) {
	fmt.Fprintf(&l.buf, format+"\n", args...)
}

func (l *memoryLogger) String() string {
	return l.buf.String()
}
