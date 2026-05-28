package file

import (
	"os"
	"path/filepath"
)

// Mkdir 创建单级目录。父目录不存在时返回错误。
func Mkdir(path string, opts ...DirOption) error {
	cfg := defaultDirConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	if err := os.Mkdir(path, cfg.perm); err != nil {
		return &PathError{Op: "mkdir", Path: path, Err: err}
	}
	return nil
}

// MkdirAll 递归创建目录，已存在时幂等返回。
func MkdirAll(path string, opts ...DirOption) error {
	cfg := defaultDirConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	if err := os.MkdirAll(path, cfg.perm); err != nil {
		return &PathError{Op: "mkdir", Path: path, Err: err}
	}
	return nil
}

// EnsureDir 确保目录存在，不存在时创建。
func EnsureDir(path string, opts ...DirOption) error {
	return MkdirAll(path, opts...)
}

// RemoveEmpty 删除空目录。目录非空时返回错误。
func RemoveEmpty(path string) error {
	err := os.Remove(path)
	if err != nil {
		return &PathError{Op: "remove", Path: path, Err: err}
	}
	return nil
}

// CleanDir 清空目录内容，保留目录本身。
func CleanDir(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return &PathError{Op: "clean", Path: path, Err: err}
	}
	for _, entry := range entries {
		p := filepath.Join(path, entry.Name())
		if err := os.RemoveAll(p); err != nil {
			return &PathError{Op: "clean", Path: p, Err: err}
		}
	}
	return nil
}

// List 列出目录下的直接子项（不递归）。返回文件名列表。
func List(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, &PathError{Op: "list", Path: path, Err: err}
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

// ListFiles 列出目录下的直接文件（不含子目录）。
func ListFiles(path string) ([]*FileInfo, error) {
	return listByFilter(path, false)
}

// ListDirs 列出目录下的直接子目录。
func ListDirs(path string) ([]*FileInfo, error) {
	return listByFilter(path, true)
}

func listByFilter(path string, dirs bool) ([]*FileInfo, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, &PathError{Op: "list", Path: path, Err: err}
	}
	var result []*FileInfo
	for _, e := range entries {
		if e.IsDir() != dirs {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, &PathError{Op: "list", Path: path, Err: err}
		}
		result = append(result, FromStdFileInfo(info, path))
	}
	return result, nil
}

// ListAll 列出目录下的所有直接子项（含文件和目录）。
func ListAll(path string) ([]*FileInfo, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, &PathError{Op: "list", Path: path, Err: err}
	}
	result := make([]*FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			return nil, &PathError{Op: "list", Path: path, Err: err}
		}
		result = append(result, FromStdFileInfo(info, path))
	}
	return result, nil
}

// DirSize 计算目录的总大小（递归，字节）。
func DirSize(path string) (int64, error) {
	var size int64
	err := filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			size += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, &PathError{Op: "size", Path: path, Err: err}
	}
	return size, nil
}

// Count 统计目录下的文件数量（递归）。
func Count(path string) (int64, error) {
	var count int64
	err := filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	if err != nil {
		return 0, &PathError{Op: "count", Path: path, Err: err}
	}
	return count, nil
}

// Size 计算文件或目录大小（字节）。目录递归计算。
func Size(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, &PathError{Op: "size", Path: path, Err: err}
	}
	if !fi.IsDir() {
		return fi.Size(), nil
	}
	return DirSize(path)
}

// CountFiles 是 Count 的别名，保持 API 一致性。
func CountFiles(path string) (int64, error) {
	return Count(path)
}
