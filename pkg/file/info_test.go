package file

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestStat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	fi, err := Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if fi.Name != "test.txt" {
		t.Fatalf("Stat().Name = %q, want %q", fi.Name, "test.txt")
	}
	if fi.Size != 5 {
		t.Fatalf("Stat().Size = %d, want 5", fi.Size)
	}
	if fi.IsDir {
		t.Fatal("Stat().IsDir = true, want false")
	}
}

func TestStatDir(t *testing.T) {
	dir := t.TempDir()
	fi, err := Stat(dir)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !fi.IsDir {
		t.Fatal("Stat().IsDir = false, want true")
	}
}

func TestLStat(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	os.WriteFile(src, []byte("x"), 0644)
	link := filepath.Join(dir, "link.txt")
	os.Symlink(src, link)

	fi, err := LStat(link)
	if err != nil {
		t.Fatalf("LStat() error = %v", err)
	}
	if fi.Name != "link.txt" {
		t.Fatalf("LStat().Name = %q, want %q", fi.Name, "link.txt")
	}
}

func TestFileSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	size, err := FileSize(path)
	if err != nil {
		t.Fatalf("FileSize() error = %v", err)
	}
	if size != 5 {
		t.Fatalf("FileSize() = %d, want 5", size)
	}
}

func TestModTime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("x"), 0644)

	mt, err := ModTime(path)
	if err != nil {
		t.Fatalf("ModTime() error = %v", err)
	}
	if mt.IsZero() {
		t.Fatal("ModTime() returned zero time")
	}
}

func TestPermFunc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("x"), 0600)

	perm, err := Perm(path)
	if err != nil {
		t.Fatalf("Perm() error = %v", err)
	}
	if perm != 0600 {
		t.Fatalf("Perm() = %o, want 0600", perm)
	}
}

func TestChecksum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	md5, err := Checksum(path, "md5")
	if err != nil {
		t.Fatalf("Checksum(md5) error = %v", err)
	}
	if md5 == "" {
		t.Fatal("Checksum(md5) returned empty")
	}

	sha256, err := Checksum(path, "sha256")
	if err != nil {
		t.Fatalf("Checksum(sha256) error = %v", err)
	}
	if sha256 == "" {
		t.Fatal("Checksum(sha256) returned empty")
	}
	if md5 == sha256 {
		t.Fatal("md5 and sha256 should differ")
	}
}

func TestChecksumUnsupported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("x"), 0644)

	_, err := Checksum(path, "crc32")
	if err == nil {
		t.Fatal("Checksum(crc32) error = nil, want error")
	}
}

func TestEqual(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	c := filepath.Join(dir, "c.txt")
	os.WriteFile(a, []byte("hello"), 0644)
	os.WriteFile(b, []byte("hello"), 0644)
	os.WriteFile(c, []byte("world"), 0644)

	eq, err := Equal(a, b)
	if err != nil {
		t.Fatalf("Equal() error = %v", err)
	}
	if !eq {
		t.Fatal("Equal(a, b) = false, want true")
	}

	eq, err = Equal(a, c)
	if err != nil {
		t.Fatalf("Equal() error = %v", err)
	}
	if eq {
		t.Fatal("Equal(a, c) = true, want false")
	}
}

func TestFromStdFileInfo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	fi, _ := os.Stat(path)
	info := FromStdFileInfo(fi, dir)
	if info.Name != "test.txt" {
		t.Fatalf("Name = %q, want %q", info.Name, "test.txt")
	}
	if info.Path != path {
		t.Fatalf("Path = %q, want %q", info.Path, path)
	}
}

func TestFileInfoType(t *testing.T) {
	fi := &FileInfo{IsDir: true}
	if fi.Type() != "dir" {
		t.Fatalf("Type() = %q, want %q", fi.Type(), "dir")
	}
	fi = &FileInfo{IsDir: false, Mode: 0}
	if fi.Type() != "file" {
		t.Fatalf("Type() = %q, want %q", fi.Type(), "file")
	}
	fi = &FileInfo{IsDir: false, Mode: fs.ModeSocket}
	if fi.Type() != "other" {
		t.Fatalf("Type() = %q, want %q", fi.Type(), "other")
	}
}

func TestFileInfoExtension(t *testing.T) {
	fi := &FileInfo{Name: "test.go"}
	if fi.Extension() != ".go" {
		t.Fatalf("Extension() = %q, want %q", fi.Extension(), ".go")
	}
}

func TestFileInfoStem(t *testing.T) {
	fi := &FileInfo{Name: "test.go"}
	if fi.Stem() != "test" {
		t.Fatalf("Stem() = %q, want %q", fi.Stem(), "test")
	}
}
