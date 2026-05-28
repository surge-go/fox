//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd

package file

import (
	"fmt"
	"os"
)

func flock(fd *os.File, lt LockType, nonblocking bool) error {
	return fmt.Errorf("file: flock not supported on this platform")
}

func flockTry(fd *os.File, lt LockType) error {
	return fmt.Errorf("file: flock not supported on this platform")
}

func funlock(fd *os.File) error {
	return fmt.Errorf("file: flock not supported on this platform")
}
