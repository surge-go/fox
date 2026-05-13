package config

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
)

var (
	ErrConfigNotFound   = errors.New("config file not found")
	ErrConfigReadFailed = errors.New("read config failed")
)

// Config 配置管理器
type Config struct {
	viper *viper.Viper // viper 实例
	mu    sync.RWMutex // 并发保护锁

	// 配置文件相关
	configFile  string   // 配置文件完整路径
	configName  string   // 配置文件名（不含扩展名）
	configType  string   // 配置文件类型
	configPaths []string // 配置文件搜索路径

	// 监控相关
	protected bool        // 是否启用保护模式
	autoWatch bool        // 是否自动开启文件监控
	watching  bool        // 是否正在监控
	restoring atomic.Bool // 是否正在恢复配置文件
	onChange  func()      // 配置变更回调
	onError   func(error) // 错误回调
	snap      *snapshot   // 配置文件快照

	// 其他选项
	defaults       map[string]any    // 默认配置值
	envPrefix      string            // 环境变量前缀
	envKeyReplacer *strings.Replacer // 环境变量键名替换器
}

// 全局默认实例
var (
	defaultInstance *Config
	defaultMu       sync.RWMutex
)

// Default 获取全局默认配置实例
// 如果未通过 SetDefault 设置，则自动创建一个空实例
func Default() *Config {
	defaultMu.RLock()
	if defaultInstance != nil {
		defer defaultMu.RUnlock()
		return defaultInstance
	}
	defaultMu.RUnlock()

	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultInstance == nil {
		defaultInstance = &Config{
			viper: viper.New(),
		}
	}
	return defaultInstance
}

// SetDefault 设置全局默认配置实例
func SetDefault(c *Config) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultInstance = c
}

// New 创建新的配置管理器
func New(opts ...Option) *Config {
	c := &Config{
		viper: viper.New(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Load 加载配置文件
func (c *Config) Load() error {
	c.mu.Lock()

	// 设置默认值
	for k, v := range c.defaults {
		c.viper.SetDefault(k, v)
	}

	// 设置环境变量
	if c.envPrefix != "" {
		c.viper.SetEnvPrefix(c.envPrefix)
		c.viper.AutomaticEnv()
	}
	if c.envKeyReplacer != nil {
		c.viper.SetEnvKeyReplacer(c.envKeyReplacer)
	}

	// 设置配置文件
	if c.configFile != "" {
		c.viper.SetConfigFile(c.configFile)
	} else {
		if c.configName != "" {
			c.viper.SetConfigName(c.configName)
		}
		if c.configType != "" {
			c.viper.SetConfigType(c.configType)
		}
		for _, path := range c.configPaths {
			c.viper.AddConfigPath(path)
		}
	}

	// 读取配置文件
	if err := c.viper.ReadInConfig(); err != nil {
		c.mu.Unlock()
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return fmt.Errorf("%w: %w", ErrConfigNotFound, err)
		}
		return fmt.Errorf("%w: %w", ErrConfigReadFailed, err)
	}

	// 保护模式下保存快照
	var snapErr error
	if c.protected {
		snapErr = c.saveSnapshot()
	}

	// 自动开启监控
	if c.autoWatch {
		c.startWatch()
	}

	c.mu.Unlock()

	// 释放锁后报告快照错误，避免在锁内调用用户回调
	if snapErr != nil {
		c.reportError(snapErr)
	}

	return nil
}

// Get 泛型获取配置值
func Get[T any](c *Config, key string) T {
	c.mu.RLock()
	defer c.mu.RUnlock()

	val := c.viper.Get(key)
	if val == nil {
		var zero T
		return zero
	}

	if v, ok := val.(T); ok {
		return v
	}

	var zero T
	return zero
}

// GetString 获取字符串配置值
func (c *Config) GetString(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.viper.GetString(key)
}

// GetInt 获取整数配置值
func (c *Config) GetInt(key string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.viper.GetInt(key)
}

// GetInt64 获取 int64 配置值
func (c *Config) GetInt64(key string) int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.viper.GetInt64(key)
}

// GetFloat64 获取 float64 配置值
func (c *Config) GetFloat64(key string) float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.viper.GetFloat64(key)
}

// GetBool 获取布尔配置值
func (c *Config) GetBool(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.viper.GetBool(key)
}

// GetDuration 获取时间间隔配置值
func (c *Config) GetDuration(key string) time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.viper.GetDuration(key)
}

// GetStringSlice 获取字符串切片配置值
func (c *Config) GetStringSlice(key string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.viper.GetStringSlice(key)
}

// GetStringMap 获取字符串映射配置值
func (c *Config) GetStringMap(key string) map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.viper.GetStringMap(key)
}

// GetStringMapString 获取字符串到字符串的映射配置值
func (c *Config) GetStringMapString(key string) map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.viper.GetStringMapString(key)
}

// Set 设置配置值
func (c *Config) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.viper.Set(key, value)
}

// IsSet 检查配置键是否存在
func (c *Config) IsSet(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.viper.IsSet(key)
}

// AllSettings 获取所有配置
func (c *Config) AllSettings() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.viper.AllSettings()
}

// Sub 获取子配置
// 返回一个新的 Config 实例，包含指定 key 下的所有配置
// 注意：返回的实例为只读轻量实例，不继承监控、保护模式等属性
func (c *Config) Sub(key string) *Config {
	c.mu.RLock()
	defer c.mu.RUnlock()

	sub := c.viper.Sub(key)
	if sub == nil {
		return nil
	}

	return &Config{
		viper: sub,
	}
}

// Unmarshal 将配置反序列化到结构体
func (c *Config) Unmarshal(rawVal any) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.viper.Unmarshal(rawVal)
}

// UnmarshalKey 将指定 key 的配置反序列化到结构体
func (c *Config) UnmarshalKey(key string, rawVal any) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.viper.UnmarshalKey(key, rawVal)
}

// Close 关闭配置管理器，停止监控并释放资源
func (c *Config) Close() {
	c.StopWatch()
}

// Viper 获取底层 viper 实例（用于高级操作）
// 注意：直接操作 viper 实例不受 Config 的并发锁保护，需自行确保线程安全
func (c *Config) Viper() *viper.Viper {
	return c.viper
}
