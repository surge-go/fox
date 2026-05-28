//go:build darwin || linux || freebsd || openbsd || netbsd

package file

import (
	"os"
	"syscall"
)

func flock(fd *os.File, lt LockType, nonblocking bool) error {
	var how int
	switch lt {
	case ReadLock:
		how = syscall.LOCK_SH
	case WriteLock:
		how = syscall.LOCK_EX
	default:
		how = syscall.LOCK_EX
	}
	if nonblocking {
		how |= syscall.LOCK_NB
	}
	return syscall.Flock(int(fd.Fd()), how)
}

func flockTry(fd *os.File, lt LockType) error {
	return flock(fd, lt, true)
}

func funlock(fd *os.File) error {
	return syscall.Flock(int(fd.Fd()), syscall.LOCK_UN)
}
