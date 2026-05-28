package file

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	data, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("ReadFile() = %q, want %q", data, "hello")
	}
}

func TestReadFileNotExist(t *testing.T) {
	_, err := ReadFile("/nonexistent/path")
	if err == nil {
		t.Fatal("ReadFile() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "read") {
		t.Fatalf("ReadFile() error = %v, want to contain 'read'", err)
	}
}

func TestReadString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	got, err := ReadString(path)
	if err != nil {
		t.Fatalf("ReadString() error = %v", err)
	}
	if got != "hello world" {
		t.Fatalf("ReadString() = %q, want %q", got, "hello world")
	}
}

func TestReadLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lines.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3"), 0644)

	lines, err := ReadLines(path)
	if err != nil {
		t.Fatalf("ReadLines() error = %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("ReadLines() got %d lines, want 3", len(lines))
	}
	if lines[0] != "line1" || lines[1] != "line2" || lines[2] != "line3" {
		t.Fatalf("ReadLines() = %v, want [line1 line2 line3]", lines)
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "test.txt")

	if err := WriteFile(path, []byte("data"), WithPerm(0600)); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "data" {
		t.Fatalf("content = %q, want %q", data, "data")
	}
}

func TestWriteString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if err := WriteString(path, "hello"); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "hello" {
		t.Fatalf("content = %q, want %q", data, "hello")
	}
}

func TestWriteLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if err := WriteLine(path, "hello"); err != nil {
		t.Fatalf("WriteLine() error = %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "hello\n" {
		t.Fatalf("content = %q, want %q", data, "hello\n")
	}
}

func TestAppendFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("line1"), 0644)

	if err := AppendFile(path, []byte("\nline2")); err != nil {
		t.Fatalf("AppendFile() error = %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "line1\nline2" {
		t.Fatalf("content = %q, want %q", data, "line1\nline2")
	}
}

func TestAppendLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("line1"), 0644)

	if err := AppendLine(path, "line2"); err != nil {
		t.Fatalf("AppendLine() error = %v", err)
	}

	data, _ := os.ReadFile(path)
	expected := "line1line2\n"
	if string(data) != expected {
		t.Fatalf("content = %q, want %q", data, expected)
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "sub", "dst.txt")
	os.WriteFile(src, []byte("content"), 0644)

	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile() error = %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "content" {
		t.Fatalf("dst content = %q, want %q", data, "content")
	}
}

func TestCopyFileNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("new"), 0644)
	os.WriteFile(dst, []byte("old"), 0644)

	err := CopyFile(src, dst, WithNoOverwrite())
	if err == nil {
		t.Fatal("CopyFile() error = nil, want error")
	}
	if !IsExist(err) {
		t.Fatalf("CopyFile() error = %v, want ErrExist", err)
	}
}

func TestCopyDir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.MkdirAll(filepath.Join(src, "sub"), 0755)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("b"), 0644)

	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir() error = %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dst, "a.txt"))
	if string(data) != "a" {
		t.Fatalf("a.txt = %q, want %q", data, "a")
	}
	data, _ = os.ReadFile(filepath.Join(dst, "sub", "b.txt"))
	if string(data) != "b" {
		t.Fatalf("b.txt = %q, want %q", data, "b")
	}
}

func TestCopyDirRejectsDestinationInsideSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(src, "backup")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := CopyDir(src, dst)
	if err == nil {
		t.Fatal("CopyDir() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "inside source") {
		t.Fatalf("CopyDir() error = %v, want inside source", err)
	}
}

func TestCopyDirRejectsSamePath(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	err := CopyDir(src, src)
	if err == nil {
		t.Fatal("CopyDir() error = nil, want error")
	}
}

func TestMoveFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("data"), 0644)

	if err := MoveFile(src, dst); err != nil {
		t.Fatalf("MoveFile() error = %v", err)
	}

	if Exists(src) {
		t.Fatal("source file still exists after move")
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "data" {
		t.Fatalf("dst content = %q, want %q", data, "data")
	}
}

func TestMoveDir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.MkdirAll(src, 0755)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0644)

	if err := MoveDir(src, dst); err != nil {
		t.Fatalf("MoveDir() error = %v", err)
	}

	if Exists(src) {
		t.Fatal("source dir still exists after move")
	}
	data, _ := os.ReadFile(filepath.Join(dst, "a.txt"))
	if string(data) != "a" {
		t.Fatalf("content = %q, want %q", data, "a")
	}
}

func TestRemoveFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("x"), 0644)

	if err := RemoveFile(path); err != nil {
		t.Fatalf("RemoveFile() error = %v", err)
	}
	if Exists(path) {
		t.Fatal("file still exists after remove")
	}
}

func TestRemoveFileNotExist(t *testing.T) {
	err := RemoveFile("/nonexistent/path")
	if err != nil {
		t.Fatalf("RemoveFile() error = %v, want nil", err)
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("x"), 0644)

	if !Exists(path) {
		t.Fatal("Exists() = false, want true")
	}
	if Exists(filepath.Join(dir, "nope.txt")) {
		t.Fatal("Exists() = true for nonexistent, want false")
	}
}

func TestIsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("x"), 0644)

	if !IsFile(path) {
		t.Fatal("IsFile() = false, want true")
	}
	if IsFile(dir) {
		t.Fatal("IsFile() = true for dir, want false")
	}
}

func TestIsDir(t *testing.T) {
	dir := t.TempDir()
	if !IsDir(dir) {
		t.Fatal("IsDir() = false, want true")
	}
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("x"), 0644)
	if IsDir(path) {
		t.Fatal("IsDir() = true for file, want false")
	}
}

func TestIsEmpty(t *testing.T) {
	dir := t.TempDir()

	empty, err := IsEmpty(dir)
	if err != nil {
		t.Fatalf("IsEmpty() error = %v", err)
	}
	if !empty {
		t.Fatal("IsEmpty() = false for empty dir, want true")
	}

	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0644)
	empty, err = IsEmpty(dir)
	if err != nil {
		t.Fatalf("IsEmpty() error = %v", err)
	}
	if empty {
		t.Fatal("IsEmpty() = true for non-empty dir, want false")
	}

	path := filepath.Join(dir, "empty.txt")
	os.WriteFile(path, []byte(""), 0644)
	empty, err = IsEmpty(path)
	if err != nil {
		t.Fatalf("IsEmpty() error = %v", err)
	}
	if !empty {
		t.Fatal("IsEmpty() = false for empty file, want true")
	}
}

func TestIsSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	os.WriteFile(src, []byte("x"), 0644)
	link := filepath.Join(dir, "link.txt")
	os.Symlink(src, link)

	if !IsSymlink(link) {
		t.Fatal("IsSymlink() = false for symlink, want true")
	}
	if IsSymlink(src) {
		t.Fatal("IsSymlink() = true for regular file, want false")
	}
}

func TestRemoveFileRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "subdir")
	os.Mkdir(target, 0755)

	err := RemoveFile(target)
	if err == nil {
		t.Fatal("RemoveFile() on directory error = nil, want error")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Fatalf("RemoveFile() error = %v, want to contain 'directory'", err)
	}
}

func TestReadLinesLongLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "long.txt")
	long := strings.Repeat("a", 200*1024) // 200KB line
	os.WriteFile(path, []byte(long+"\nshort"), 0644)

	lines, err := ReadLines(path)
	if err != nil {
		t.Fatalf("ReadLines() error = %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("ReadLines() got %d lines, want 2", len(lines))
	}
	if len(lines[0]) != 200*1024 {
		t.Fatalf("ReadLines() first line length = %d, want %d", len(lines[0]), 200*1024)
	}
}

func TestCopyFilePreserveMode(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("data"), 0755)

	// Default: preserve source mode
	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile() error = %v", err)
	}
	dstInfo, _ := os.Stat(dst)
	if dstInfo.Mode().Perm() != 0755 {
		t.Fatalf("CopyFile() perm = %o, want 0755", dstInfo.Mode().Perm())
	}

	// With explicit perm
	dst2 := filepath.Join(dir, "dst2.txt")
	if err := CopyFile(src, dst2, WithCopyPerm(0600)); err != nil {
		t.Fatalf("CopyFile() error = %v", err)
	}
	dst2Info, _ := os.Stat(dst2)
	if dst2Info.Mode().Perm() != 0600 {
		t.Fatalf("CopyFile() perm = %o, want 0600", dst2Info.Mode().Perm())
	}
}
