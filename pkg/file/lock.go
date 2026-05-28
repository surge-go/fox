package file

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// LockType 锁类型。
type LockType int

const (
	ReadLock  LockType = iota // 共享读锁
	WriteLock                 // 独占写锁
)

// FileLock 进程级文件锁。所有方法均为并发安全。
type FileLock struct {
	path   string
	mu     sync.Mutex // 保护 fd 字段
	fd     *os.File
	locked atomic.Bool
}

// NewFileLock 创建文件锁。锁文件的父目录会自动创建。
func NewFileLock(path string) (*FileLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, &PathError{Op: "lock", Path: path, Err: err}
	}
	return &FileLock{path: path}, nil
}

// Acquire 获取锁。WriteLock 为独占锁，ReadLock 为共享锁。阻塞直到获取成功。
func (fl *FileLock) Acquire(lockType LockType) error {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	if err := fl.openLocked(); err != nil {
		return err
	}

	if err := flock(fl.fd, lockType, false); err != nil {
		return &PathError{Op: "lock", Path: fl.path, Err: err}
	}
	fl.locked.Store(true)
	return nil
}

// TryAcquire 尝试获取锁，获取失败时立即返回错误（不阻塞）。
func (fl *FileLock) TryAcquire(lockType LockType) error {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	if err := fl.openLocked(); err != nil {
		return err
	}

	if err := flockTry(fl.fd, lockType); err != nil {
		return &PathError{Op: "lock", Path: fl.path, Err: err}
	}
	fl.locked.Store(true)
	return nil
}

// AcquireWithTimeout 在超时时间内尝试获取锁。非 EWOULDBLOCK 错误会立即返回。
func (fl *FileLock) AcquireWithTimeout(lockType LockType, timeout time.Duration) error {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	if err := fl.openLocked(); err != nil {
		return err
	}

	deadline := time.Now().Add(timeout)
	backoff := 5 * time.Millisecond
	const maxBackoff = 100 * time.Millisecond
	for {
		err := flockTry(fl.fd, lockType)
		if err == nil {
			fl.locked.Store(true)
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return &PathError{Op: "lock", Path: fl.path, Err: err}
		}
		if time.Now().After(deadline) {
			return &PathError{Op: "lock", Path: fl.path, Err: fmt.Errorf("timeout after %s", timeout)}
		}
		time.Sleep(backoff)
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// Release 释放锁。重复调用是安全的（幂等）。
func (fl *FileLock) Release() error {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	if fl.fd == nil || !fl.locked.Load() {
		return nil
	}
	fl.locked.Store(false)
	if err := funlock(fl.fd); err != nil {
		return &PathError{Op: "unlock", Path: fl.path, Err: err}
	}
	return nil
}

// Close 释放锁并关闭文件描述符。重复调用是安全的。
func (fl *FileLock) Close() error {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	fd := fl.fd
	fl.fd = nil

	if fd == nil {
		return nil
	}
	var firstErr error
	if fl.locked.Load() {
		fl.locked.Store(false)
		if err := funlock(fd); err != nil {
			firstErr = &PathError{Op: "unlock", Path: fl.path, Err: err}
		}
	}
	if err := fd.Close(); err != nil && firstErr == nil {
		firstErr = &PathError{Op: "close", Path: fl.path, Err: err}
	}
	return firstErr
}

func (fl *FileLock) openLocked() error {
	if fl.fd != nil {
		return nil
	}
	fd, err := os.OpenFile(fl.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return &PathError{Op: "open", Path: fl.path, Err: err}
	}
	fl.fd = fd
	return nil
}
