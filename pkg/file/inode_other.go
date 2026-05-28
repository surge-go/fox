//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd

package file

// inodeKey 在不支持 inode 的平台上使用路径作为键。
func inodeKey(path string) (string, error) {
	return path, nil
}
