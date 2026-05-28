package file

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMkdir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "newdir")

	if err := Mkdir(target); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if !IsDir(target) {
		t.Fatal("Mkdir() dir not created")
	}
}

func TestMkdirAll(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a", "b", "c")

	if err := MkdirAll(target); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if !IsDir(target) {
		t.Fatal("MkdirAll() dir not created")
	}
}

func TestMkdirAllIdempotent(t *testing.T) {
	dir := t.TempDir()
	if err := MkdirAll(dir); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
}

func TestEnsureDir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x", "y")
	if err := EnsureDir(target); err != nil {
		t.Fatalf("EnsureDir() error = %v", err)
	}
	if !IsDir(target) {
		t.Fatal("EnsureDir() dir not created")
	}
}

func TestRemoveEmpty(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "empty")
	os.Mkdir(target, 0755)

	if err := RemoveEmpty(target); err != nil {
		t.Fatalf("RemoveEmpty() error = %v", err)
	}
	if Exists(target) {
		t.Fatal("dir still exists after RemoveEmpty")
	}
}

func TestRemoveEmptyNonEmpty(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "notempty")
	os.Mkdir(target, 0755)
	os.WriteFile(filepath.Join(target, "a.txt"), []byte("x"), 0644)

	err := RemoveEmpty(target)
	if err == nil {
		t.Fatal("RemoveEmpty() error = nil for non-empty dir, want error")
	}
}

func TestCleanDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("b"), 0644)

	if err := CleanDir(dir); err != nil {
		t.Fatalf("CleanDir() error = %v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("CleanDir() dir has %d entries, want 0", len(entries))
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)
	os.Mkdir(filepath.Join(dir, "sub"), 0755)

	names, err := List(dir)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(names) != 3 {
		t.Fatalf("List() got %d items, want 3", len(names))
	}
}

func TestListFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.Mkdir(filepath.Join(dir, "sub"), 0755)

	files, err := ListFiles(dir)
	if err != nil {
		t.Fatalf("ListFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("ListFiles() got %d files, want 1", len(files))
	}
	if files[0].Name != "a.txt" {
		t.Fatalf("ListFiles()[0].Name = %q, want %q", files[0].Name, "a.txt")
	}
}

func TestListDirs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.Mkdir(filepath.Join(dir, "sub"), 0755)

	dirs, err := ListDirs(dir)
	if err != nil {
		t.Fatalf("ListDirs() error = %v", err)
	}
	if len(dirs) != 1 {
		t.Fatalf("ListDirs() got %d dirs, want 1", len(dirs))
	}
	if dirs[0].Name != "sub" {
		t.Fatalf("ListDirs()[0].Name = %q, want %q", dirs[0].Name, "sub")
	}
}

func TestListAll(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.Mkdir(filepath.Join(dir, "sub"), 0755)

	all, err := ListAll(dir)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("ListAll() got %d items, want 2", len(all))
	}
}

func TestDirSize(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("world"), 0644)

	size, err := DirSize(dir)
	if err != nil {
		t.Fatalf("DirSize() error = %v", err)
	}
	if size != 10 {
		t.Fatalf("DirSize() = %d, want 10", size)
	}
}

func TestCount(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "c.txt"), []byte("c"), 0644)

	count, err := Count(dir)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("Count() = %d, want 3", count)
	}
}

func TestSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	size, err := Size(path)
	if err != nil {
		t.Fatalf("Size() error = %v", err)
	}
	if size != 5 {
		t.Fatalf("Size() = %d, want 5", size)
	}
}
