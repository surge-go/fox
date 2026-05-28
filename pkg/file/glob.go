package file

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GlobResult 单次 Glob 匹配结果。
type GlobResult struct {
	Path    string    // 匹配的相对路径
	AbsPath string    // 匹配的绝对路径
	Info    *FileInfo // 文件信息
}

// GlobOption Glob 选项。
type GlobOption func(*globConfig)

type globConfig struct {
	maxDepth int
	onlyFile bool
	onlyDir  bool
}

func defaultGlobConfig() *globConfig {
	return &globConfig{}
}

// WithGlobMaxDepth 设置 Glob 最大搜索深度。
func WithGlobMaxDepth(depth int) GlobOption {
	return func(c *globConfig) {
		c.maxDepth = depth
	}
}

// WithGlobFilesOnly 设置 Glob 只匹配文件。
func WithGlobFilesOnly() GlobOption {
	return func(c *globConfig) {
		c.onlyFile = true
	}
}

// WithGlobDirsOnly 设置 Glob 只匹配目录。
func WithGlobDirsOnly() GlobOption {
	return func(c *globConfig) {
		c.onlyDir = true
	}
}

// Glob 在指定目录下匹配 glob 模式，返回匹配的文件列表。支持 ** 递归匹配子目录。
func Glob(root string, pattern string, opts ...GlobOption) ([]*GlobResult, error) {
	cfg := defaultGlobConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.onlyFile && cfg.onlyDir {
		return nil, &PathError{Op: "glob", Path: root, Err: fmt.Errorf("WithGlobFilesOnly and WithGlobDirsOnly are mutually exclusive")}
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, &PathError{Op: "glob", Path: root, Err: err}
	}

	parts := splitPattern(pattern)
	var results []*GlobResult

	if err := globWalk(rootAbs, "", parts, cfg, 0, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// Match 判断文件名是否匹配 glob 模式。
func Match(pattern, name string) (bool, error) {
	return filepath.Match(pattern, name)
}

func splitPattern(pattern string) []string {
	pattern = filepath.ToSlash(pattern)
	parts := strings.Split(pattern, "/")
	return parts
}

func globWalk(absRoot, relPath string, parts []string, cfg *globConfig, depth int, results *[]*GlobResult) error {
	if len(parts) == 0 {
		return nil
	}

	if cfg.maxDepth > 0 && depth > cfg.maxDepth {
		return nil
	}

	part := parts[0]
	remaining := parts[1:]

	absCurrent := absRoot
	if relPath != "" {
		absCurrent = filepath.Join(absRoot, relPath)
	}

	if part == "**" {
		// ** 匹配零个或多个目录
		// 尝试匹配剩余模式在当前层级
		if err := globMatchDepth(absRoot, relPath, remaining, cfg, depth, results); err != nil {
			return err
		}
		// 递归进入子目录
		entries, err := os.ReadDir(absCurrent)
		if err != nil {
			return &PathError{Op: "glob", Path: absCurrent, Err: err}
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			childRel := e.Name()
			if relPath != "" {
				childRel = filepath.Join(relPath, e.Name())
			}
			if err := globWalk(absRoot, childRel, parts, cfg, depth+1, results); err != nil {
				return err
			}
		}
		return nil
	}

	// 普通匹配：列出当前目录，用 filepath.Match 匹配 part
	entries, err := os.ReadDir(absCurrent)
	if err != nil {
		return &PathError{Op: "glob", Path: absCurrent, Err: err}
	}

	for _, e := range entries {
		matched, err := filepath.Match(part, e.Name())
		if err != nil {
			return &PathError{Op: "glob", Path: absCurrent, Err: err}
		}
		if !matched {
			continue
		}

		childRel := e.Name()
		if relPath != "" {
			childRel = filepath.Join(relPath, e.Name())
		}

		if len(remaining) == 0 {
			// 叶子节点，收集结果
			if cfg.onlyFile && e.IsDir() {
				continue
			}
			if cfg.onlyDir && !e.IsDir() {
				continue
			}

			info, err := e.Info()
			if err != nil {
				continue
			}
			*results = append(*results, &GlobResult{
				Path:    childRel,
				AbsPath: filepath.Join(absRoot, childRel),
				Info: &FileInfo{
					Name:    info.Name(),
					Path:    filepath.Join(absRoot, childRel),
					Size:    info.Size(),
					Mode:    info.Mode(),
					ModTime: info.ModTime(),
					IsDir:   info.IsDir(),
				},
			})
		} else if e.IsDir() {
			// 中间路径段，继续递归
			if err := globWalk(absRoot, childRel, remaining, cfg, depth+1, results); err != nil {
				return err
			}
		}
	}

	return nil
}

func globMatch(absRoot, relPath string, parts []string, cfg *globConfig, results *[]*GlobResult) error {
	return globMatchDepth(absRoot, relPath, parts, cfg, 0, results)
}

func globMatchDepth(absRoot, relPath string, parts []string, cfg *globConfig, depth int, results *[]*GlobResult) error {
	if cfg.maxDepth > 0 && depth > cfg.maxDepth {
		return nil
	}

	if len(parts) == 0 {
		// 模式已匹配完，收集当前路径
		absPath := absRoot
		if relPath != "" {
			absPath = filepath.Join(absRoot, relPath)
		}
		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return &PathError{Op: "glob", Path: absPath, Err: err}
		}
		if cfg.onlyFile && info.IsDir() {
			return nil
		}
		if cfg.onlyDir && !info.IsDir() {
			return nil
		}
		*results = append(*results, &GlobResult{
			Path:    relPath,
			AbsPath: absPath,
			Info: &FileInfo{
				Name:    info.Name(),
				Path:    absPath,
				Size:    info.Size(),
				Mode:    info.Mode(),
				ModTime: info.ModTime(),
				IsDir:   info.IsDir(),
			},
		})
		return nil
	}

	part := parts[0]
	remaining := parts[1:]

	absCurrent := absRoot
	if relPath != "" {
		absCurrent = filepath.Join(absRoot, relPath)
	}

	if part == "**" {
		if err := globMatchDepth(absRoot, relPath, remaining, cfg, depth, results); err != nil {
			return err
		}
		entries, err := os.ReadDir(absCurrent)
		if err != nil {
			return &PathError{Op: "glob", Path: absCurrent, Err: err}
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			childRel := e.Name()
			if relPath != "" {
				childRel = filepath.Join(relPath, e.Name())
			}
			if err := globMatchDepth(absRoot, childRel, parts, cfg, depth+1, results); err != nil {
				return err
			}
		}
		return nil
	}

	entries, err := os.ReadDir(absCurrent)
	if err != nil {
		return &PathError{Op: "glob", Path: absCurrent, Err: err}
	}

	for _, e := range entries {
		matched, err := filepath.Match(part, e.Name())
		if err != nil {
			return &PathError{Op: "glob", Path: absCurrent, Err: err}
		}
		if !matched {
			continue
		}

		childRel := e.Name()
		if relPath != "" {
			childRel = filepath.Join(relPath, e.Name())
		}

		if len(remaining) == 0 {
			if cfg.onlyFile && e.IsDir() {
				continue
			}
			if cfg.onlyDir && !e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			*results = append(*results, &GlobResult{
				Path:    childRel,
				AbsPath: filepath.Join(absRoot, childRel),
				Info: &FileInfo{
					Name:    info.Name(),
					Path:    filepath.Join(absRoot, childRel),
					Size:    info.Size(),
					Mode:    info.Mode(),
					ModTime: info.ModTime(),
					IsDir:   info.IsDir(),
				},
			})
		} else if e.IsDir() {
			if err := globMatchDepth(absRoot, childRel, remaining, cfg, depth+1, results); err != nil {
				return err
			}
		}
	}

	return nil
}
