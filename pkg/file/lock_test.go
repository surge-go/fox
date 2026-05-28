package file

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestFileLockAcquireRelease(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	fl, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer fl.Close()

	if err := fl.Acquire(WriteLock); err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if err := fl.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
}

func TestFileLockClose(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	fl, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}

	if err := fl.Acquire(WriteLock); err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if err := fl.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestFileLockReadLockShared(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	fl1, _ := NewFileLock(lockPath)
	fl2, _ := NewFileLock(lockPath)
	defer fl1.Close()
	defer fl2.Close()

	if err := fl1.Acquire(ReadLock); err != nil {
		t.Fatalf("fl1.Acquire(ReadLock) error = %v", err)
	}
	if err := fl2.Acquire(ReadLock); err != nil {
		t.Fatalf("fl2.Acquire(ReadLock) error = %v", err)
	}
}

func TestFileLockTryAcquire(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	fl1, _ := NewFileLock(lockPath)
	fl2, _ := NewFileLock(lockPath)
	defer fl1.Close()
	defer fl2.Close()

	if err := fl1.Acquire(WriteLock); err != nil {
		t.Fatalf("fl1.Acquire() error = %v", err)
	}

	err := fl2.TryAcquire(WriteLock)
	if err == nil {
		t.Fatal("TryAcquire() error = nil, want error (lock held by fl1)")
	}
}

func TestFileLockReleaseNoLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	fl, _ := NewFileLock(lockPath)
	defer fl.Close()

	// Release without acquire should be fine
	if err := fl.Release(); err != nil {
		t.Fatalf("Release() error = %v, want nil", err)
	}
}

func TestNewFileLockCreatesDir(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "sub", "deep", "test.lock")

	fl, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer fl.Close()

	if !IsDir(filepath.Dir(lockPath)) {
		t.Fatal("parent dir not created")
	}
}

func TestFileLockConcurrentCloseAndRelease(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	fl, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	if err := fl.Acquire(WriteLock); err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = fl.Release()
		}()
		go func() {
			defer wg.Done()
			_ = fl.Close()
		}()
	}
	wg.Wait()

	if err := fl.Close(); err != nil {
		t.Fatalf("Close() after concurrent use error = %v", err)
	}
}
