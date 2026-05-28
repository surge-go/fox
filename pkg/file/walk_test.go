package file

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWalkVisitsRoot(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)

	var rootVisited bool
	err := Walk(dir, func(entry *WalkEntry) error {
		if entry.Depth == 0 && entry.Path == "" {
			rootVisited = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
	if !rootVisited {
		t.Fatal("Walk() did not visit root")
	}
}

func TestWalkMaxDepth1(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a", "b"), 0755)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "a", "deep.txt"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "a", "b", "deeper.txt"), []byte("x"), 0644)

	var files []string
	err := Walk(dir, func(entry *WalkEntry) error {
		if entry.Info != nil && !entry.Info.IsDir {
			files = append(files, entry.Path)
		}
		return nil
	}, WithMaxDepth(1))
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
	// maxDepth=1: root (depth 0) + direct children (depth 1). No deeper.
	for _, f := range files {
		if filepath.Dir(f) != "" && filepath.Dir(f) != "." {
			// This file is in a subdirectory beyond depth 1
			depth := len(filepath.SplitList(f))
			if depth > 1 {
				t.Fatalf("Walk(maxDepth=1) visited %q which is too deep", f)
			}
		}
	}
}

func TestWalkFilesSkipsNilInfo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)

	var files []string
	err := WalkFiles(dir, func(entry *WalkEntry) error {
		files = append(files, entry.AbsPath)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("WalkFiles() got %d files, want 1", len(files))
	}
}

func TestWalkDirsSkipsNilInfo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)

	var dirs []string
	err := WalkDirs(dir, func(entry *WalkEntry) error {
		dirs = append(dirs, entry.AbsPath)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDirs() error = %v", err)
	}
	// Should include root + subdir, but not a.txt
	if len(dirs) < 2 {
		t.Fatalf("WalkDirs() got %d dirs, want >= 2", len(dirs))
	}
}

func TestWalkSkipDirs(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "skip", "nested"), 0755)
	os.MkdirAll(filepath.Join(dir, "keep"), 0755)
	os.WriteFile(filepath.Join(dir, "keep", "a.txt"), []byte("a"), 0644)

	var count int
	err := Walk(dir, func(entry *WalkEntry) error {
		if entry.Info != nil && !entry.Info.IsDir {
			count++
		}
		return nil
	}, WithSkipDirs("skip"))
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("Walk() got %d files, want 1 (only from keep/)", count)
	}
}

func TestWalkSkipFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.log"), []byte("b"), 0644)

	var paths []string
	err := Walk(dir, func(entry *WalkEntry) error {
		if entry.Info != nil && !entry.Info.IsDir {
			paths = append(paths, entry.Path)
		}
		return nil
	}, WithSkipFiles("b.log"))
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
	for _, p := range paths {
		if p == "b.log" {
			t.Fatalf("Walk() visited skipped file: %q", p)
		}
	}
}

func TestWalkPattern(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)

	var paths []string
	err := Walk(dir, func(entry *WalkEntry) error {
		if entry.Info != nil && !entry.Info.IsDir {
			paths = append(paths, entry.Path)
		}
		return nil
	}, WithPattern("*.go"))
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("Walk() got %d files, want 1", len(paths))
	}
	if paths[0] != "a.go" {
		t.Fatalf("Walk() got %q, want %q", paths[0], "a.go")
	}
}
