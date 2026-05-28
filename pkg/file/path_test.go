package file

import (
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"double_slash", "/tmp//foo", "/tmp/foo"},
		{"dot_dot", "/tmp/foo/../bar", "/tmp/bar"},
		{"trailing_slash", "/tmp/foo/", "/tmp/foo"},
		{"single_dot", "/tmp/./foo", "/tmp/foo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Clean(tt.input)
			if got != tt.want {
				t.Fatalf("Clean(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExt(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"go_file", "main.go", ".go"},
		{"yaml_file", "config.yaml", ".yaml"},
		{"no_ext", "Makefile", ""},
		{"path", "/tmp/data.json", ".json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Ext(tt.input)
			if got != tt.want {
				t.Fatalf("Ext(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBasename(t *testing.T) {
	got := Basename("/tmp/foo/bar.txt")
	if got != "bar.txt" {
		t.Fatalf("Basename = %q, want %q", got, "bar.txt")
	}
}

func TestStem(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"with_ext", "config.yaml", "config"},
		{"no_ext", "Makefile", "Makefile"},
		{"path", "/tmp/data.json", "data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Stem(tt.input)
			if got != tt.want {
				t.Fatalf("Stem(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDir(t *testing.T) {
	got := Dir("/tmp/foo/bar.txt")
	if got != "/tmp/foo" {
		t.Fatalf("Dir = %q, want %q", got, "/tmp/foo")
	}
}

func TestJoin(t *testing.T) {
	got := Join("/tmp", "foo", "bar.txt")
	if got != "/tmp/foo/bar.txt" {
		t.Fatalf("Join = %q, want %q", got, "/tmp/foo/bar.txt")
	}
}

func TestAbs(t *testing.T) {
	got, err := Abs(".")
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	if !strings.HasPrefix(got, "/") {
		t.Fatalf("Abs = %q, want absolute path", got)
	}
}

func TestRel(t *testing.T) {
	got, err := Rel("/tmp/foo", "/tmp/foo/bar/baz")
	if err != nil {
		t.Fatalf("Rel() error = %v", err)
	}
	if got != "bar/baz" {
		t.Fatalf("Rel = %q, want %q", got, "bar/baz")
	}
}

func TestHome(t *testing.T) {
	got, err := Home()
	if err != nil {
		t.Fatalf("Home() error = %v", err)
	}
	if got == "" {
		t.Fatal("Home() returned empty string")
	}
}

func TestSystemTempDir(t *testing.T) {
	got := SystemTempDir()
	if got == "" {
		t.Fatal("SystemTempDir() returned empty string")
	}
}

func TestEnsureExt(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		ext   string
		want  string
	}{
	{"add_ext", "config", ".yaml", "config.yaml"},
	{"already_has", "config.yaml", ".yaml", "config.yaml"},
	{"add_without_dot", "config", "yaml", "config.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EnsureExt(tt.path, tt.ext)
			if got != tt.want {
				t.Fatalf("EnsureExt(%q, %q) = %q, want %q", tt.path, tt.ext, got, tt.want)
			}
		})
	}
}

func TestChangeExt(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		ext   string
		want  string
	}{
	{"json_to_yaml", "data.json", ".yaml", "data.yaml"},
	{"with_dot", "data.json", "yaml", "data.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChangeExt(tt.path, tt.ext)
			if got != tt.want {
				t.Fatalf("ChangeExt(%q, %q) = %q, want %q", tt.path, tt.ext, got, tt.want)
			}
		})
	}
}
