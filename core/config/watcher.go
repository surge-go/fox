package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// snapshot 保存配置文件快照，用于保护模式下的恢复
type snapshot struct {
	content []byte
}

// startWatch 开始监控配置文件变更
func (c *Config) startWatch() {
	c.viper.OnConfigChange(func(e fsnotify.Event) {
		// 如果正在恢复中，忽略此次事件
		if c.restoring.Load() {
			return
		}

		c.mu.RLock()
		watching := c.watching
		protected := c.protected
		onChange := c.onChange
		snapContent := c.copySnapContent()
		c.mu.RUnlock()

		// 已停止监控，忽略事件
		if !watching {
			return
		}

		if protected {
			// 保护模式：恢复文件内容
			c.restoreFromContent(snapContent)
		} else {
			// 非保护模式：触发回调
			if onChange != nil {
				onChange()
			}
		}
	})
	c.viper.WatchConfig()
	c.watching = true
}

// StopWatch 停止监控配置文件
// 注意：viper 未提供停止底层 fsnotify watcher 的方法，
// 此方法仅标记状态使回调不再生效，底层 watcher 在 Config 生命周期内持续运行
func (c *Config) StopWatch() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.watching = false
}

// StartWatch 开始监控配置文件变更
// 如果已经在监控中，则不重复启动
func (c *Config) StartWatch() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.watching {
		return nil
	}

	c.startWatch()
	return nil
}

// SetProtected 设置保护模式
func (c *Config) SetProtected(protected bool) {
	c.mu.Lock()
	c.protected = protected

	// 开启保护模式时，保存当前快照
	var snapErr error
	if protected {
		snapErr = c.saveSnapshot()
	}
	c.mu.Unlock()

	// 释放锁后报告快照错误，避免在锁内调用用户回调
	if snapErr != nil {
		c.reportError(snapErr)
	}
}

// IsProtected 查询是否处于保护模式
func (c *Config) IsProtected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.protected
}

// saveSnapshot 保存当前配置文件快照
// 注意：调用方必须已持有 mu 锁，此方法内不再加锁
// 返回错误供调用方在释放锁后报告，避免在锁内调用用户回调导致死锁
func (c *Config) saveSnapshot() error {
	file := c.viper.ConfigFileUsed()
	if file == "" {
		return nil
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("保存快照失败: %w", err)
	}

	c.snap = &snapshot{content: data}
	return nil
}

// copySnapContent 在锁保护下复制快照内容
// 调用方必须持有 mu.RLock
func (c *Config) copySnapContent() []byte {
	if c.snap == nil {
		return nil
	}
	cp := make([]byte, len(c.snap.content))
	copy(cp, c.snap.content)
	return cp
}

// restoreFromContent 使用给定内容恢复配置文件
// 使用临时文件 + 原子替换确保可靠性
func (c *Config) restoreFromContent(content []byte) {
	if content == nil {
		return
	}

	file := c.viper.ConfigFileUsed()
	if file == "" {
		return
	}

	// 标记正在恢复，防止恢复写入触发二次事件
	c.restoring.Store(true)
	defer c.restoring.Store(false)

	dir := filepath.Dir(file)
	tmp, err := os.CreateTemp(dir, ".config-restore-*")
	if err != nil {
		c.reportError(fmt.Errorf("创建临时文件失败: %w", err))
		return
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		c.reportError(fmt.Errorf("写入临时文件失败: %w", err))
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		c.reportError(fmt.Errorf("关闭临时文件失败: %w", err))
		return
	}

	if err := os.Rename(tmpName, file); err != nil {
		os.Remove(tmpName)
		c.reportError(fmt.Errorf("恢复配置文件失败: %w", err))
		return
	}

	// 恢复文件后重新读取，确保 viper 内存状态与文件一致
	c.mu.Lock()
	err = c.viper.ReadInConfig()
	c.mu.Unlock()
	if err != nil {
		c.reportError(fmt.Errorf("恢复后重新加载配置失败: %w", err))
	}
}

// reportError 报告错误，优先使用 onError 回调，否则输出到 stderr
func (c *Config) reportError(err error) {
	c.mu.RLock()
	onError := c.onError
	c.mu.RUnlock()

	if onError != nil {
		onError(err)
	} else {
		fmt.Fprintf(os.Stderr, "[config] %v\n", err)
	}
}
