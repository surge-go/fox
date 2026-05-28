//go:build darwin || linux || freebsd || openbsd || netbsd

package file

import (
	"fmt"
	"os"
	"syscall"
)

// inodeKey 返回路径的 (dev, inode) 组合键，用于检测符号链接循环。
func inodeKey(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return path, nil
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}
