package bootstrap

import (
	"errors"

	"github.com/surge-go/fox/core/database"
	"github.com/surge-go/fox/core/logger"
	"github.com/surge-go/fox/core/metrics"
	"github.com/surge-go/fox/core/redis"
	"github.com/surge-go/fox/core/server"
	"github.com/surge-go/fox/core/tracing"
)

// Config 是 fox 应用门面使用的顶层配置。
//
// 每个字段都对应一个已有的 core 包，调用方既可以单独初始化这些包，
// 也可以交给根包统一编排。
type Config struct {
	Logger  *logger.Config  `json:"logger,omitempty" yaml:"logger,omitempty" mapstructure:"logger"`
	Tracing *tracing.Config `json:"tracing,omitempty" yaml:"tracing,omitempty" mapstructure:"tracing"`
	Metrics *metrics.Config `json:"metrics,omitempty" yaml:"metrics,omitempty" mapstructure:"metrics"`

	Server   *server.Config   `json:"server,omitempty" yaml:"server,omitempty" mapstructure:"server"`
	Database *database.Config `json:"database,omitempty" yaml:"database,omitempty" mapstructure:"database"`
	Redis    *redis.Config    `json:"redis,omitempty" yaml:"redis,omitempty" mapstructure:"redis"`
}

// Validate 校验顶层配置，并把各包自身的配置校验委托给对应的 core 配置类型。
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("fox config is nil")
	}

	var errs []error
	if c.Logger != nil {
		errs = append(errs, c.Logger.Validate())
	}
	if c.Tracing != nil {
		errs = append(errs, c.Tracing.Validate())
	}
	if c.Metrics != nil {
		errs = append(errs, c.Metrics.Validate())
	}
	if c.Server != nil {
		errs = append(errs, c.Server.Validate())
	}
	if c.Database != nil {
		errs = append(errs, c.Database.Validate())
	}
	if c.Redis != nil {
		errs = append(errs, c.Redis.Validate())
	}
	return errors.Join(errs...)
}
