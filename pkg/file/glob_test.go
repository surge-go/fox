package file

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGlobBasic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("b"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("c"), 0644)

	results, err := Glob(dir, "*.go")
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Glob() got %d results, want 2", len(results))
	}
}

func TestGlobRecursive(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "b.go"), []byte("b"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("c"), 0644)

	results, err := Glob(dir, "**/*.go")
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Glob() got %d results, want 2", len(results))
	}
}

func TestGlobFilesOnly(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)

	results, err := Glob(dir, "*", WithGlobFilesOnly())
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Glob() got %d results, want 1", len(results))
	}
}

func TestGlobDirsOnly(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)

	results, err := Glob(dir, "*", WithGlobDirsOnly())
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Glob() got %d results, want 1", len(results))
	}
}

func TestGlobMaxDepth(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0755)
	os.WriteFile(filepath.Join(dir, "a", "shallow.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "a", "b", "deep.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "a", "b", "c", "deeper.go"), []byte("x"), 0644)

	results, err := Glob(dir, "**/*.go", WithGlobMaxDepth(2))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	// a/shallow.go at depth 1 and a/b/deep.go at depth 2 are within maxDepth=2
	// a/b/c/deeper.go at depth 3 is beyond maxDepth
	if len(results) != 2 {
		t.Fatalf("Glob() got %d results, want 2", len(results))
	}
}

func TestGlobMaxDepthAppliesAcrossNestedGlobstars(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0755)
	os.WriteFile(filepath.Join(dir, "a", "shallow.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "a", "b", "deep.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "a", "b", "c", "deeper.go"), []byte("x"), 0644)

	results, err := Glob(dir, "**/**/*.go", WithGlobMaxDepth(2))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	for _, result := range results {
		if result.Path == filepath.Join("a", "b", "c", "deeper.go") {
			t.Fatalf("Glob() returned %q beyond max depth", result.Path)
		}
	}
}

func TestGlobEmpty(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)

	results, err := Glob(dir, "*.go")
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("Glob() got %d results, want 0", len(results))
	}
}

func TestMatch(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		file    string
		want    bool
	}{
		{"exact", "test.go", "test.go", true},
		{"star", "*.go", "main.go", true},
		{"no_match", "*.go", "main.txt", false},
		{"question", "?.txt", "a.txt", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Match(tt.pattern, tt.file)
			if err != nil {
				t.Fatalf("Match() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Match(%q, %q) = %v, want %v", tt.pattern, tt.file, got, tt.want)
			}
		})
	}
}

func TestGlobResultPath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("x"), 0644)

	results, err := Glob(dir, "*.go")
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Glob() got %d results, want 1", len(results))
	}
	if results[0].Path != "test.go" {
		t.Fatalf("GlobResult.Path = %q, want %q", results[0].Path, "test.go")
	}
	if results[0].AbsPath != filepath.Join(dir, "test.go") {
		t.Fatalf("GlobResult.AbsPath = %q, want %q", results[0].AbsPath, filepath.Join(dir, "test.go"))
	}
}
