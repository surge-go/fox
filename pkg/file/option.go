package file

import "io/fs"

// WriteOption 文件写入选项。
type WriteOption func(*writeConfig)

type writeConfig struct {
	perm    fs.FileMode
	dirPerm fs.FileMode
}

func defaultWriteConfig() *writeConfig {
	return &writeConfig{
		perm:    0644,
		dirPerm: 0755,
	}
}

// WithPerm 设置文件写入权限。
func WithPerm(perm fs.FileMode) WriteOption {
	return func(c *writeConfig) {
		c.perm = perm
	}
}

// WithDirPerm 设置自动创建的父目录权限。
func WithDirPerm(perm fs.FileMode) WriteOption {
	return func(c *writeConfig) {
		c.dirPerm = perm
	}
}

// CopyOption 文件复制选项。
type CopyOption func(*copyConfig)

type copyConfig struct {
	perm         fs.FileMode
	dirPerm      fs.FileMode
	preserveMode bool
	overwrite    bool
}

func defaultCopyConfig() *copyConfig {
	return &copyConfig{
		dirPerm:      0755,
		preserveMode: true,
		overwrite:    true,
	}
}

// WithCopyPerm 设置复制后的目标文件权限（覆盖源文件权限）。
func WithCopyPerm(perm fs.FileMode) CopyOption {
	return func(c *copyConfig) {
		c.perm = perm
		c.preserveMode = false
	}
}

// WithNoOverwrite 禁止覆盖已存在的目标文件。
func WithNoOverwrite() CopyOption {
	return func(c *copyConfig) {
		c.overwrite = false
	}
}

// DirOption 目录操作选项。
type DirOption func(*dirConfig)

type dirConfig struct {
	perm fs.FileMode
}

func defaultDirConfig() *dirConfig {
	return &dirConfig{
		perm: 0755,
	}
}

// WithDirPermission 设置目录创建权限。
func WithDirPermission(perm fs.FileMode) DirOption {
	return func(c *dirConfig) {
		c.perm = perm
	}
}

// TempOption 临时文件/目录选项。
type TempOption func(*tempConfig)

type tempConfig struct {
	prefix string
	suffix string
	dir    string
	ext    string
}

func defaultTempConfig() *tempConfig {
	return &tempConfig{}
}

// WithTempPrefix 设置临时文件/目录名前缀。
func WithTempPrefix(prefix string) TempOption {
	return func(c *tempConfig) {
		c.prefix = prefix
	}
}

// WithTempDir 设置临时文件/目录的父目录。
func WithTempDir(dir string) TempOption {
	return func(c *tempConfig) {
		c.dir = dir
	}
}

// WithTempSuffix 设置临时文件/目录名后缀。
func WithTempSuffix(suffix string) TempOption {
	return func(c *tempConfig) {
		c.suffix = suffix
	}
}

// WithTempExt 设置临时文件扩展名。
func WithTempExt(ext string) TempOption {
	return func(c *tempConfig) {
		c.ext = ext
	}
}
