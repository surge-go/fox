package file

import (
	"os"
	"path/filepath"
)

// TempFile 在指定目录创建临时文件，返回文件句柄。调用方负责关闭文件。
func TempFile(opts ...TempOption) (*os.File, error) {
	cfg := defaultTempConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	dir := cfg.dir
	if dir == "" {
		dir = os.TempDir()
	}

	name := cfg.prefix + "*" + cfg.suffix + cfg.ext
	f, err := os.CreateTemp(dir, name)
	if err != nil {
		return nil, &PathError{Op: "temp", Path: dir, Err: err}
	}
	return f, nil
}

// TempFilePath 创建临时文件并立即关闭，返回文件路径。
func TempFilePath(opts ...TempOption) (string, error) {
	f, err := TempFile(opts...)
	if err != nil {
		return "", err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		return "", &PathError{Op: "temp", Path: path, Err: err}
	}
	return path, nil
}

// TempDirPath 在指定目录创建临时目录，返回目录路径。
func TempDirPath(opts ...TempOption) (string, error) {
	cfg := defaultTempConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	dir := cfg.dir
	if dir == "" {
		dir = os.TempDir()
	}

	name := cfg.prefix + "*" + cfg.suffix
	path, err := os.MkdirTemp(dir, name)
	if err != nil {
		return "", &PathError{Op: "tempdir", Path: dir, Err: err}
	}
	return path, nil
}

// TempDirWithCleanup 创建临时目录，返回目录路径和清理函数。
func TempDirWithCleanup(opts ...TempOption) (string, func() error, error) {
	path, err := TempDirPath(opts...)
	if err != nil {
		return "", nil, err
	}
	cleanup := func() error {
		return os.RemoveAll(path)
	}
	return path, cleanup, nil
}

// TempFileWithCleanup 创建临时文件，返回文件路径和清理函数。文件在创建后立即关闭。
func TempFileWithCleanup(opts ...TempOption) (string, func() error, error) {
	f, err := TempFile(opts...)
	if err != nil {
		return "", nil, err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		return "", nil, &PathError{Op: "temp", Path: path, Err: err}
	}
	cleanup := func() error {
		return os.Remove(path)
	}
	return path, cleanup, nil
}

// RemoveTemp 清理临时文件或目录。
func RemoveTemp(path string) error {
	return os.RemoveAll(path)
}

// GlobTemp 匹配临时目录下的文件模式。
func GlobTemp(pattern string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), pattern))
	if err != nil {
		return nil, &PathError{Op: "glob", Path: os.TempDir(), Err: err}
	}
	return matches, nil
}
