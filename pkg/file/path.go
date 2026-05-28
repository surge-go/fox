package file

import (
	"os"
	"path/filepath"
	"strings"
)

// Clean 清理路径，去除多余分隔符和 . 和 ..。
func Clean(path string) string {
	return filepath.Clean(path)
}

// Ext 返回文件扩展名（含点号），例如 ".go"。
func Ext(path string) string {
	return filepath.Ext(path)
}

// Basename 返回文件名（含扩展名）。
func Basename(path string) string {
	return filepath.Base(path)
}

// Stem 返回文件名（不含扩展名）。
func Stem(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(path)
	return strings.TrimSuffix(base, ext)
}

// Dir 返回父目录路径。
func Dir(path string) string {
	return filepath.Dir(path)
}

// Join 拼接路径片段。
func Join(elem ...string) string {
	return filepath.Join(elem...)
}

// Abs 返回绝对路径。相对路径基于当前工作目录。
func Abs(path string) (string, error) {
	return filepath.Abs(path)
}

// Rel 返回从 base 到 target 的相对路径。
func Rel(base, target string) (string, error) {
	return filepath.Rel(base, target)
}

// Home 返回当前用户主目录。
func Home() (string, error) {
	return os.UserHomeDir()
}

// SystemTempDir 返回系统临时目录。
func SystemTempDir() string {
	return os.TempDir()
}

// EnsureExt 确保文件路径具有指定扩展名，没有时追加。
func EnsureExt(path, ext string) string {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	if filepath.Ext(path) == ext {
		return path
	}
	return path + ext
}

// ChangeExt 更改文件路径的扩展名。
func ChangeExt(path, ext string) string {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	old := filepath.Ext(path)
	return strings.TrimSuffix(path, old) + ext
}
