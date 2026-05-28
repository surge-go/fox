package file

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// SkipDir 是 WalkFunc 返回的特殊错误，用于跳过当前目录。
var SkipDir = errors.New("file: skip directory")

// WalkEntry 遍历目录时的单个条目。
type WalkEntry struct {
	Path    string    // 相对于根目录的路径
	AbsPath string    // 绝对路径
	Info    *FileInfo // 文件信息
	Depth   int       // 相对根目录的深度（根目录为 0）
	Error   error     // 遍历此条目时遇到的错误
}

// WalkFunc 遍历回调函数。返回 SkipDir 跳过当前目录。
type WalkFunc func(entry *WalkEntry) error

// WalkOption 遍历选项。
type WalkOption func(*walkConfig)

type walkConfig struct {
	maxDepth  int
	follow    bool
	skipDirs  []string
	skipFiles []string
	patterns  []string
}

func defaultWalkConfig() *walkConfig {
	return &walkConfig{}
}

// WithMaxDepth 设置最大遍历深度。0 表示不限制。1 表示只遍历直接子项。
func WithMaxDepth(depth int) WalkOption {
	return func(c *walkConfig) {
		c.maxDepth = depth
	}
}

// WithFollowSymlinks 设置是否跟随符号链接。
func WithFollowSymlinks() WalkOption {
	return func(c *walkConfig) {
		c.follow = true
	}
}

// WithSkipDirs 设置需要跳过的目录名列表。
func WithSkipDirs(dirs ...string) WalkOption {
	return func(c *walkConfig) {
		c.skipDirs = append(c.skipDirs, dirs...)
	}
}

// WithSkipFiles 设置需要跳过的文件名列表。
func WithSkipFiles(files ...string) WalkOption {
	return func(c *walkConfig) {
		c.skipFiles = append(c.skipFiles, files...)
	}
}

// WithPattern 设置 glob 匹配模式，只遍历匹配的文件。仅支持单层匹配，不支持 ** 递归。
func WithPattern(pattern string) WalkOption {
	return func(c *walkConfig) {
		c.patterns = append(c.patterns, pattern)
	}
}

// Walk 递归遍历目录。根目录本身也会被访问。
func Walk(root string, fn WalkFunc, opts ...WalkOption) error {
	cfg := defaultWalkConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return &PathError{Op: "walk", Path: root, Err: err}
	}

	// Visit root entry first.
	var rootInfo *FileInfo
	if cfg.follow {
		fi, err := os.Stat(rootAbs)
		if err != nil {
			return &PathError{Op: "walk", Path: rootAbs, Err: err}
		}
		rootInfo = &FileInfo{
			Name:    fi.Name(),
			Path:    rootAbs,
			Size:    fi.Size(),
			Mode:    fi.Mode(),
			ModTime: fi.ModTime(),
			IsDir:   fi.IsDir(),
		}
	} else {
		fi, err := os.Lstat(rootAbs)
		if err != nil {
			return &PathError{Op: "walk", Path: rootAbs, Err: err}
		}
		rootInfo = &FileInfo{
			Name:    fi.Name(),
			Path:    rootAbs,
			Size:    fi.Size(),
			Mode:    fi.Mode(),
			ModTime: fi.ModTime(),
			IsDir:   fi.IsDir(),
		}
	}

	rootEntry := &WalkEntry{
		Path:    "",
		AbsPath: rootAbs,
		Info:    rootInfo,
		Depth:   0,
	}
	if err := fn(rootEntry); err != nil {
		if errors.Is(err, SkipDir) || errors.Is(err, fs.SkipDir) {
			return nil
		}
		return err
	}

	if rootInfo.IsDir {
		visited := make(map[string]bool)
		if cfg.follow {
			if key, err := inodeKey(rootAbs); err == nil {
				visited[key] = true
			}
		}
		return walkDir(rootAbs, "", fn, cfg, 0, visited)
	}
	return nil
}

// WalkFiles 只遍历文件，跳过目录。带错误信息的条目也会跳过。
func WalkFiles(root string, fn WalkFunc, opts ...WalkOption) error {
	return Walk(root, func(entry *WalkEntry) error {
		if entry.Info == nil || entry.Info.IsDir {
			return nil
		}
		return fn(entry)
	}, opts...)
}

// WalkDirs 只遍历目录。带错误信息的条目也会跳过。
func WalkDirs(root string, fn WalkFunc, opts ...WalkOption) error {
	return Walk(root, func(entry *WalkEntry) error {
		if entry.Info == nil || !entry.Info.IsDir {
			return nil
		}
		return fn(entry)
	}, opts...)
}

func walkDir(absRoot, relPath string, fn WalkFunc, cfg *walkConfig, depth int, visited map[string]bool) error {
	absPath := filepath.Join(absRoot, relPath)

	// maxDepth > 0: depth >= maxDepth means we don't recurse into children.
	// depth 0 = root's children, depth 1 = grandchildren, etc.
	if cfg.maxDepth > 0 && depth >= cfg.maxDepth {
		return nil
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		entry := &WalkEntry{
			Path:    relPath,
			AbsPath: absPath,
			Depth:   depth + 1,
			Error:   err,
		}
		return fn(entry)
	}

	for _, e := range entries {
		entryRel := e.Name()
		if relPath != "" {
			entryRel = filepath.Join(relPath, e.Name())
		}
		entryAbs := filepath.Join(absRoot, entryRel)

		if e.IsDir() {
			if slices.Contains(cfg.skipDirs, e.Name()) {
				continue
			}
		} else {
			if slices.Contains(cfg.skipFiles, e.Name()) {
				continue
			}
			if len(cfg.patterns) > 0 {
				matched := false
				for _, p := range cfg.patterns {
					if ok, _ := filepath.Match(p, e.Name()); ok {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
		}

		var info *FileInfo
		var infoErr error
		if cfg.follow {
			fi, err := os.Stat(entryAbs)
			if err != nil {
				infoErr = err
			} else {
				info = &FileInfo{
					Name:    fi.Name(),
					Path:    entryAbs,
					Size:    fi.Size(),
					Mode:    fi.Mode(),
					ModTime: fi.ModTime(),
					IsDir:   fi.IsDir(),
				}
			}
		} else {
			fi, err := e.Info()
			if err != nil {
				infoErr = err
			} else {
				info = FromStdFileInfo(fi, filepath.Dir(entryAbs))
				info.Path = entryAbs
			}
		}

		if infoErr != nil {
			entry := &WalkEntry{
				Path:    entryRel,
				AbsPath: entryAbs,
				Depth:   depth + 1,
				Error:   infoErr,
			}
			if err := fn(entry); err != nil {
				return err
			}
			continue
		}

		entry := &WalkEntry{
			Path:    entryRel,
			AbsPath: entryAbs,
			Info:    info,
			Depth:   depth + 1,
		}

		if err := fn(entry); err != nil {
			if errors.Is(err, SkipDir) {
				continue
			}
			return err
		}

		if info.IsDir {
			if cfg.follow {
				key, err := inodeKey(entryAbs)
				if err == nil && visited[key] {
					continue // 检测到符号链接循环，跳过
				}
				if err == nil {
					visited[key] = true
				}
			}
			if err := walkDir(absRoot, entryRel, fn, cfg, depth+1, visited); err != nil {
				if errors.Is(err, fs.SkipDir) {
					continue
				}
				return err
			}
		}
	}

	return nil
}
