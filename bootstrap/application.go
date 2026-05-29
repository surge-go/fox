package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"sync"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/surge-go/fox/core/database"
	"github.com/surge-go/fox/core/logger"
	"github.com/surge-go/fox/core/metrics"
	"github.com/surge-go/fox/core/redis"
	"github.com/surge-go/fox/core/server"
	"github.com/surge-go/fox/core/tracing"
)

// HookFunc 生命周期钩子函数。
type HookFunc func(ctx context.Context) error

// Application 应用门面，统一编排各 core 模块的初始化和生命周期。
type Application struct {
	cfg *Config

	logger  *zap.Logger
	tracing *tracing.Provider
	metrics *metrics.Provider
	db      *gorm.DB
	redis   goredis.UniversalClient
	server  *server.Engine

	startHooks  []HookFunc
	stopHooks   []HookFunc
	mu          sync.Mutex
	once        sync.Once // 保证 shutdown 只执行一次
	shutdownErr error
}

// New 创建 Application。按顺序初始化：Logger → Tracing → Metrics → Database → Redis → Server。
// 任意步骤失败时逆序关闭已初始化的模块。
func New(cfg *Config) (*Application, error) {
	if cfg == nil {
		return nil, fmt.Errorf("bootstrap: config must not be nil")
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("bootstrap: config validation failed: %w", err)
	}

	cfgCopy := *cfg
	app := &Application{cfg: &cfgCopy}

	// 1. Logger
	if cfg.Logger != nil {
		l, err := logger.New(cfg.Logger)
		if err != nil {
			return nil, fmt.Errorf("bootstrap: init logger: %w", err)
		}
		app.logger = l
	}

	// 2. Tracing
	if cfg.Tracing != nil {
		p, err := tracing.New(context.Background(), cfg.Tracing)
		if err != nil {
			return nil, app.initError("tracing", err)
		}
		app.tracing = p
	}

	// 3. Metrics
	if cfg.Metrics != nil {
		p, err := metrics.New(context.Background(), cfg.Metrics)
		if err != nil {
			return nil, app.initError("metrics", err)
		}
		app.metrics = p
	}

	// 4. Database
	if cfg.Database != nil {
		var (
			db  *gorm.DB
			err error
		)
		if app.logger != nil {
			db, err = database.NewClientWithLogger(cfg.Database, app.logger)
		} else {
			db, err = database.NewClient(cfg.Database)
		}
		if err != nil {
			return nil, app.initError("database", err)
		}
		app.db = db
	}

	// 5. Redis
	if cfg.Redis != nil {
		r, err := redis.NewClient(cfg.Redis)
		if err != nil {
			return nil, app.initError("redis", err)
		}
		app.redis = r
	}

	// 6. Server
	if cfg.Server != nil {
		s, err := server.New(cfg.Server)
		if err != nil {
			return nil, app.initError("server", err)
		}
		app.server = s
	}

	return app, nil
}

func (a *Application) initError(component string, err error) error {
	return errors.Join(
		fmt.Errorf("bootstrap: init %s: %w", component, err),
		a.shutdown(),
	)
}

// OnStart 注册启动钩子。在 Server.Run 之前按注册顺序执行。
func (a *Application) OnStart(fn HookFunc) {
	if fn == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.startHooks = append(a.startHooks, fn)
}

// OnStop 注册停止钩子。在 Server.Shutdown 之后按注册逆序执行。
func (a *Application) OnStop(fn HookFunc) {
	if fn == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopHooks = append(a.stopHooks, fn)
}

// Run 启动应用。执行启动钩子后启动 Server（阻塞，监听信号后优雅关闭）。
func (a *Application) Run() error {
	// 复制 hooks 切片，避免 data race
	a.mu.Lock()
	hooks := make([]HookFunc, len(a.startHooks))
	copy(hooks, a.startHooks)
	a.mu.Unlock()

	// 执行启动钩子
	ctx := context.Background()
	for _, fn := range hooks {
		if err := fn(ctx); err != nil {
			shutdownErr := a.shutdown()
			return errors.Join(
				fmt.Errorf("bootstrap: start hook failed: %w", err),
				shutdownErr,
			)
		}
	}

	// 启动 Server（阻塞，内置信号处理和 graceful shutdown）
	if a.server != nil {
		if err := a.server.Run(); err != nil {
			shutdownErr := a.shutdown()
			return errors.Join(
				fmt.Errorf("bootstrap: server run: %w", err),
				shutdownErr,
			)
		}
	}

	// Server 退出后执行清理
	return a.shutdown()
}

// Shutdown 优雅关闭应用。server 完全停止后，逆序执行停止钩子，然后关闭所有模块。幂等。
func (a *Application) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	a.once.Do(func() {
		var errs []error
		if a.server != nil {
			if err := a.server.Shutdown(ctx); err != nil {
				errs = append(errs, fmt.Errorf("bootstrap: server shutdown: %w", err))
			}
		}
		errs = append(errs, a.cleanResources(ctx))
		a.shutdownErr = errors.Join(errs...)
	})
	return a.shutdownErr
}

// shutdown 幂等关闭（不传入外部 context，用于 Run 内部和 New 失败时的清理）。
func (a *Application) shutdown() error {
	a.once.Do(func() {
		a.shutdownErr = a.cleanResources(context.Background())
	})
	return a.shutdownErr
}

// cleanResources 逆序执行停止钩子并关闭所有已初始化的模块。只被 once.Do 调用。
func (a *Application) cleanResources(ctx context.Context) error {
	// 复制 stop hooks，避免 data race
	a.mu.Lock()
	hooks := make([]HookFunc, len(a.stopHooks))
	copy(hooks, a.stopHooks)
	a.mu.Unlock()

	var errs []error

	// 执行停止钩子（逆序）
	for i := len(hooks) - 1; i >= 0; i-- {
		if err := hooks[i](ctx); err != nil {
			errs = append(errs, fmt.Errorf("bootstrap: stop hook failed: %w", err))
		}
	}

	if a.redis != nil {
		if err := a.redis.Close(); err != nil {
			errs = append(errs, fmt.Errorf("bootstrap: close redis: %w", err))
		}
	}

	if a.db != nil {
		sqlDB, err := a.db.DB()
		if err != nil {
			errs = append(errs, fmt.Errorf("bootstrap: get database handle: %w", err))
		} else if err := sqlDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("bootstrap: close database: %w", err))
		}
	}

	if a.metrics != nil {
		if err := a.metrics.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("bootstrap: shutdown metrics: %w", err))
		}
	}

	if a.tracing != nil {
		if err := a.tracing.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("bootstrap: shutdown tracing: %w", err))
		}
	}

	if a.logger != nil {
		if err := a.logger.Sync(); err != nil {
			errs = append(errs, fmt.Errorf("bootstrap: sync logger: %w", err))
		}
	}

	return errors.Join(errs...)
}

// ===== 访问器 =====

// Logger 返回日志实例。未配置时返回 zap.L()。
func (a *Application) Logger() *zap.Logger {
	if a.logger != nil {
		return a.logger
	}
	return zap.L()
}

// Server 返回 HTTP 引擎。未配置时返回 nil。
func (a *Application) Server() *server.Engine {
	return a.server
}

// DB 返回数据库实例。未配置时返回 nil。
func (a *Application) DB() *gorm.DB {
	return a.db
}

// Redis 返回 Redis 客户端。未配置时返回 nil。
func (a *Application) Redis() goredis.UniversalClient {
	return a.redis
}

// Tracing 返回链路追踪 Provider。未配置时返回 nil。
func (a *Application) Tracing() *tracing.Provider {
	return a.tracing
}

// Metrics 返回指标 Provider。未配置时返回 nil。
func (a *Application) Metrics() *metrics.Provider {
	return a.metrics
}
