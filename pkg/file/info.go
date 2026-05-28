package file

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileInfo 文件信息的扩展封装。
type FileInfo struct {
	Name    string      // 文件名（不含路径）
	Path    string      // 完整路径
	Size    int64       // 文件大小（字节）
	Mode    fs.FileMode // 文件权限
	ModTime time.Time   // 最后修改时间
	IsDir   bool        // 是否为目录
}

// FromStdFileInfo 从标准库 fs.FileInfo 构建。
func FromStdFileInfo(fi fs.FileInfo, dir string) *FileInfo {
	return &FileInfo{
		Name:    fi.Name(),
		Path:    filepath.Join(dir, fi.Name()),
		Size:    fi.Size(),
		Mode:    fi.Mode(),
		ModTime: fi.ModTime(),
		IsDir:   fi.IsDir(),
	}
}

// Type 返回文件类型描述：file、dir、symlink、other。
func (fi *FileInfo) Type() string {
	if fi.IsDir {
		return "dir"
	}
	if fi.Mode&fs.ModeSymlink != 0 {
		return "symlink"
	}
	if fi.Mode.IsRegular() {
		return "file"
	}
	return "other"
}

// Extension 返回文件扩展名（含点号），例如 ".go"。
func (fi *FileInfo) Extension() string {
	return filepath.Ext(fi.Name)
}

// Stem 返回不含扩展名的文件名。
func (fi *FileInfo) Stem() string {
	ext := filepath.Ext(fi.Name)
	return strings.TrimSuffix(fi.Name, ext)
}

// Stat 获取文件信息。
func Stat(path string) (*FileInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, &PathError{Op: "stat", Path: path, Err: err}
	}
	return &FileInfo{
		Name:    fi.Name(),
		Path:    path,
		Size:    fi.Size(),
		Mode:    fi.Mode(),
		ModTime: fi.ModTime(),
		IsDir:   fi.IsDir(),
	}, nil
}

// LStat 获取文件信息（不跟随符号链接）。
func LStat(path string) (*FileInfo, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, &PathError{Op: "lstat", Path: path, Err: err}
	}
	return &FileInfo{
		Name:    fi.Name(),
		Path:    path,
		Size:    fi.Size(),
		Mode:    fi.Mode(),
		ModTime: fi.ModTime(),
		IsDir:   fi.IsDir(),
	}, nil
}

// FileSize 获取文件大小（字节）。
func FileSize(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, &PathError{Op: "stat", Path: path, Err: err}
	}
	return fi.Size(), nil
}

// ModTime 获取文件最后修改时间。
func ModTime(path string) (time.Time, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}, &PathError{Op: "stat", Path: path, Err: err}
	}
	return fi.ModTime(), nil
}

// Perm 获取文件权限。
func Perm(path string) (fs.FileMode, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, &PathError{Op: "stat", Path: path, Err: err}
	}
	return fi.Mode().Perm(), nil
}

// Checksum 计算文件的 hash 值。支持 md5、sha1、sha256。
func Checksum(path string, algo string) (string, error) {
	var h hash.Hash
	switch strings.ToLower(algo) {
	case "md5":
		h = md5.New()
	case "sha1":
		h = sha1.New()
	case "sha256":
		h = sha256.New()
	default:
		return "", &PathError{Op: "checksum", Path: path, Err: fmt.Errorf("unsupported algorithm: %s", algo)}
	}

	f, err := os.Open(path)
	if err != nil {
		return "", &PathError{Op: "checksum", Path: path, Err: err}
	}
	defer f.Close()

	if _, err := io.Copy(h, f); err != nil {
		return "", &PathError{Op: "checksum", Path: path, Err: err}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// Equal 判断两个文件内容是否相同。
func Equal(a, b string) (bool, error) {
	ha, err := Checksum(a, "sha256")
	if err != nil {
		return false, err
	}
	hb, err := Checksum(b, "sha256")
	if err != nil {
		return false, err
	}
	return ha == hb, nil
}
