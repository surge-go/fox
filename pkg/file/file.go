package file

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// PathError 路径相关错误。
type PathError struct {
	Op   string
	Path string
	Err  error
}

func (e *PathError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("file %s: %v", e.Op, e.Err)
	}
	return fmt.Sprintf("file %s %s: %v", e.Op, e.Path, e.Err)
}

func (e *PathError) Unwrap() error {
	return e.Err
}

// IsNotExist 判断错误是否为"路径不存在"。
func IsNotExist(err error) bool {
	return os.IsNotExist(err)
}

// IsExist 判断错误是否为"路径已存在"。
func IsExist(err error) bool {
	return errors.Is(err, fs.ErrExist)
}

// IsPermission 判断错误是否为"权限不足"。
func IsPermission(err error) bool {
	return os.IsPermission(err)
}

// ReadFile 读取文件全部内容。
func ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &PathError{Op: "read", Path: path, Err: err}
	}
	return data, nil
}

// ReadString 读取文件内容为字符串。
func ReadString(path string) (string, error) {
	data, err := ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ReadLines 按行读取文件内容，返回字符串切片。空行保留。
func ReadLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, &PathError{Op: "read", Path: path, Err: err}
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB max line size
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, &PathError{Op: "read", Path: path, Err: err}
	}
	return lines, nil
}

// WriteFile 写入文件，文件不存在时创建，存在时覆盖。自动创建父目录。
func WriteFile(path string, data []byte, opts ...WriteOption) error {
	cfg := defaultWriteConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, cfg.dirPerm); err != nil {
		return &PathError{Op: "write", Path: dir, Err: err}
	}
	if err := os.WriteFile(path, data, cfg.perm); err != nil {
		return &PathError{Op: "write", Path: path, Err: err}
	}
	return nil
}

// WriteString 写入字符串到文件。
func WriteString(path string, data string, opts ...WriteOption) error {
	return WriteFile(path, []byte(data), opts...)
}

// WriteLine 写入单行内容，自动追加换行符。
func WriteLine(path string, data string, opts ...WriteOption) error {
	if !strings.HasSuffix(data, "\n") {
		data += "\n"
	}
	return WriteFile(path, []byte(data), opts...)
}

// AppendFile 追加内容到文件末尾，文件不存在时创建。自动创建父目录。
func AppendFile(path string, data []byte, opts ...WriteOption) error {
	cfg := defaultWriteConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	if err := os.MkdirAll(filepath.Dir(path), cfg.dirPerm); err != nil {
		return &PathError{Op: "append", Path: path, Err: err}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, cfg.perm)
	if err != nil {
		return &PathError{Op: "append", Path: path, Err: err}
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return &PathError{Op: "append", Path: path, Err: err}
	}
	return nil
}

// AppendString 追加字符串到文件末尾。
func AppendString(path string, data string, opts ...WriteOption) error {
	return AppendFile(path, []byte(data), opts...)
}

// AppendLine 追加单行内容，自动追加换行符。
func AppendLine(path string, data string, opts ...WriteOption) error {
	if !strings.HasSuffix(data, "\n") {
		data += "\n"
	}
	return AppendFile(path, []byte(data), opts...)
}

// CopyFile 复制文件。目标父目录不存在时自动创建。
func CopyFile(src, dst string, opts ...CopyOption) error {
	cfg := defaultCopyConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return &PathError{Op: "copy", Path: src, Err: err}
	}
	if srcInfo.IsDir() {
		return &PathError{Op: "copy", Path: src, Err: fmt.Errorf("is a directory")}
	}

	if !cfg.overwrite {
		if _, statErr := os.Stat(dst); statErr == nil {
			return &PathError{Op: "copy", Path: dst, Err: fs.ErrExist}
		}
	}

	if err := os.MkdirAll(filepath.Dir(dst), cfg.dirPerm); err != nil {
		return &PathError{Op: "copy", Path: dst, Err: err}
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return &PathError{Op: "copy", Path: src, Err: err}
	}
	defer srcFile.Close()

	var perm fs.FileMode
	if cfg.preserveMode {
		perm = srcInfo.Mode().Perm()
	} else {
		perm = cfg.perm
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return &PathError{Op: "copy", Path: dst, Err: err}
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return &PathError{Op: "copy", Path: dst, Err: err}
	}
	return nil
}

// CopyDir 递归复制目录。
func CopyDir(src, dst string, opts ...CopyOption) error {
	cfg := defaultCopyConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return &PathError{Op: "copy", Path: src, Err: err}
	}
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return &PathError{Op: "copy", Path: dst, Err: err}
	}
	if sameOrSubPath(srcAbs, dstAbs) {
		return &PathError{Op: "copy", Path: dst, Err: fmt.Errorf("destination must not be source or inside source")}
	}

	srcInfo, err := os.Stat(srcAbs)
	if err != nil {
		return &PathError{Op: "copy", Path: src, Err: err}
	}
	if !srcInfo.IsDir() {
		return &PathError{Op: "copy", Path: src, Err: fmt.Errorf("not a directory")}
	}

	return filepath.WalkDir(srcAbs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return &PathError{Op: "copy", Path: path, Err: err}
		}

		relPath, err := filepath.Rel(srcAbs, path)
		if err != nil {
			return &PathError{Op: "copy", Path: path, Err: err}
		}
		dstPath := filepath.Join(dstAbs, relPath)

		if d.IsDir() {
			return os.MkdirAll(dstPath, cfg.dirPerm)
		}

		return CopyFile(path, dstPath, opts...)
	})
}

func sameOrSubPath(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if parent == child {
		return true
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != "." && rel != "" && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".."
}

// MoveFile 移动文件（重命名）。跨文件系统时自动 fallback 到复制+删除。
func MoveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return &PathError{Op: "move", Path: dst, Err: err}
	}

	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if !isCrossDevice(err) {
		return &PathError{Op: "move", Path: src, Err: err}
	}

	// Cross-filesystem: fallback to copy + remove.
	if err := CopyFile(src, dst); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil {
		return &PathError{Op: "move", Path: src, Err: fmt.Errorf("copy succeeded but remove source failed: %w", err)}
	}
	return nil
}

// MoveDir 移动目录。跨文件系统时自动 fallback 到递归复制+删除。
func MoveDir(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return &PathError{Op: "move", Path: dst, Err: err}
	}

	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if !isCrossDevice(err) {
		return &PathError{Op: "move", Path: src, Err: err}
	}

	// Cross-filesystem: fallback to copy + remove.
	if err := CopyDir(src, dst); err != nil {
		return err
	}
	if err := os.RemoveAll(src); err != nil {
		return &PathError{Op: "move", Path: src, Err: fmt.Errorf("copy succeeded but remove source failed: %w", err)}
	}
	return nil
}

// isCrossDevice 判断错误是否为跨文件系统错误（EXDEV）。
func isCrossDevice(err error) bool {
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return errors.Is(linkErr.Err, syscall.EXDEV)
	}
	return false
}

// RemoveFile 删除文件。路径为目录时返回错误。文件不存在时不报错。
func RemoveFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return &PathError{Op: "remove", Path: path, Err: err}
	}
	if info.IsDir() {
		return &PathError{Op: "remove", Path: path, Err: fmt.Errorf("is a directory, use RemoveDir")}
	}
	if err := os.Remove(path); err != nil {
		return &PathError{Op: "remove", Path: path, Err: err}
	}
	return nil
}

// RemoveDir 删除目录及其所有内容。目录不存在时不报错。
func RemoveDir(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return &PathError{Op: "remove", Path: path, Err: err}
	}
	return nil
}

// Exists 判断路径是否存在（文件或目录）。
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsFile 判断路径是否为文件（非目录）。
func IsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// IsDir 判断路径是否为目录。
func IsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// IsEmpty 判断文件或目录是否为空。
func IsEmpty(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, &PathError{Op: "stat", Path: path, Err: err}
	}

	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return false, &PathError{Op: "read", Path: path, Err: err}
		}
		return len(entries) == 0, nil
	}

	return info.Size() == 0, nil
}

// IsSymlink 判断路径是否为符号链接。
func IsSymlink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode()&fs.ModeSymlink != 0
}
