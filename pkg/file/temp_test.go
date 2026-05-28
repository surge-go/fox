package file

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTempFile(t *testing.T) {
	f, err := TempFile()
	if err != nil {
		t.Fatalf("TempFile() error = %v", err)
	}
	defer f.Close()
	defer os.Remove(f.Name())

	if f.Name() == "" {
		t.Fatal("TempFile() name is empty")
	}
}

func TestTempFileWithOptions(t *testing.T) {
	dir := t.TempDir()
	f, err := TempFile(
		WithTempDir(dir),
		WithTempPrefix("test-"),
		WithTempExt(".json"),
	)
	if err != nil {
		t.Fatalf("TempFile() error = %v", err)
	}
	name := f.Name()
	f.Close()

	if filepath.Dir(name) != dir {
		t.Fatalf("TempFile() dir = %q, want %q", filepath.Dir(name), dir)
	}
	base := filepath.Base(name)
	if len(base) < 5 || base[:5] != "test-" {
		t.Fatalf("TempFile() base = %q, want prefix 'test-'", base)
	}
}

func TestTempFilePath(t *testing.T) {
	path, err := TempFilePath()
	if err != nil {
		t.Fatalf("TempFilePath() error = %v", err)
	}
	defer os.Remove(path)

	if !Exists(path) {
		t.Fatal("TempFilePath() file does not exist")
	}
}

func TestTempDirPath(t *testing.T) {
	path, err := TempDirPath()
	if err != nil {
		t.Fatalf("TempDirPath() error = %v", err)
	}
	defer os.RemoveAll(path)

	if !IsDir(path) {
		t.Fatal("TempDirPath() is not a directory")
	}
}

func TestTempDirWithCleanup(t *testing.T) {
	path, cleanup, err := TempDirWithCleanup()
	if err != nil {
		t.Fatalf("TempDirWithCleanup() error = %v", err)
	}

	if !IsDir(path) {
		t.Fatal("TempDirWithCleanup() is not a directory")
	}

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	if Exists(path) {
		t.Fatal("dir still exists after cleanup")
	}
}

func TestTempFileWithCleanup(t *testing.T) {
	path, cleanup, err := TempFileWithCleanup()
	if err != nil {
		t.Fatalf("TempFileWithCleanup() error = %v", err)
	}

	if !Exists(path) {
		t.Fatal("file does not exist")
	}

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	if Exists(path) {
		t.Fatal("file still exists after cleanup")
	}
}

func TestRemoveTemp(t *testing.T) {
	path, err := TempDirPath()
	if err != nil {
		t.Fatalf("TempDirPath() error = %v", err)
	}

	if err := RemoveTemp(path); err != nil {
		t.Fatalf("RemoveTemp() error = %v", err)
	}
	if Exists(path) {
		t.Fatal("dir still exists after RemoveTemp")
	}
}
