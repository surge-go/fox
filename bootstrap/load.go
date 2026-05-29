package bootstrap

import (
	"fmt"
	"strings"

	coreconfig "github.com/surge-go/fox/core/config"
)

// LoadConfig 从指定配置文件加载并解析 bootstrap 配置。
//
// 该方法只读取当前配置快照，不持有 core/config 的监听生命周期。
func LoadConfig(path string, opts ...coreconfig.Option) (*Config, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("bootstrap: config file path must not be empty")
	}

	options := make([]coreconfig.Option, 0, len(opts)+1)
	options = append(options, coreconfig.WithConfigFile(path))
	options = append(options, opts...)

	return LoadConfigWithOptions(options...)
}

// LoadConfigWithOptions 使用 core/config 选项加载并解析 bootstrap 配置。
func LoadConfigWithOptions(opts ...coreconfig.Option) (*Config, error) {
	loader := coreconfig.New(opts...)
	defer loader.Close()

	if err := loader.Load(); err != nil {
		return nil, fmt.Errorf("bootstrap: load config: %w", err)
	}

	var cfg Config
	if err := loader.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("bootstrap: unmarshal config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("bootstrap: config validation failed: %w", err)
	}

	return &cfg, nil
}
