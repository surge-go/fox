package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
	"gorm.io/gorm/utils"
	"gorm.io/plugin/dbresolver"
	otelmetrics "gorm.io/plugin/opentelemetry/metrics"
	"gorm.io/plugin/opentelemetry/tracing"
)

// NewClient 根据 Config 创建 GORM 数据库客户端。
//
// 函数会先执行 Config.Validate，然后按 Driver 构造对应 GORM dialector，
// 再依次应用 GORM 初始化配置、连接池配置、读写分离和监控插件。
// 如果配置了 Logger，默认使用 zap.L() 作为 GORM 日志底层 logger。若应用没有
// 调用 zap.ReplaceGlobals 初始化全局 logger，zap.L() 默认不会输出日志。
//
// GORM 默认会在初始化时 Ping 数据库；如果应用希望延迟连接或把可用性检查交给
// readiness/health check，可以配置 GORM.DisableAutomaticPing=true。
//
// 如果插件注册或连接池初始化失败，函数会尽量关闭已经打开的底层 sql.DB，
// 避免调用方拿到部分初始化的客户端。
func NewClient(cfg *Config) (*gorm.DB, error) {
	return newClient(cfg, zap.L())
}

// NewClientWithLogger 根据 Config 和显式 zap.Logger 创建 GORM 数据库客户端。
//
// 该入口避免依赖 zap 全局 logger，适合应用启动层已经通过 internal/core/logger
// 创建好 *zap.Logger，并希望数据库日志复用同一套输出、轮转和固定字段的场景。
// log 为 nil 时使用 zap.NewNop()，不会输出日志，也不会 panic。
func NewClientWithLogger(cfg *Config, log *zap.Logger) (*gorm.DB, error) {
	return newClient(cfg, log)
}

func newClient(cfg *Config, log *zap.Logger) (*gorm.DB, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	db, err := gorm.Open(newDialector(cfg.Driver, cfg.DSN), buildGORMConfig(cfg, log))
	if err != nil {
		return nil, err
	}

	if err := applyPoolConfig(db, cfg.Pool); err != nil {
		_ = closeDB(db)
		return nil, err
	}
	if err := applyResolverConfig(db, cfg); err != nil {
		_ = closeDB(db)
		return nil, err
	}
	if err := instrumentClient(db, cfg); err != nil {
		_ = closeDB(db)
		return nil, err
	}

	return db, nil
}

// NewDB 是 NewClient 的语义化别名。
//
// 代码中如果更习惯把 GORM 对象称为 DB，可以使用 NewDB；它和 NewClient 返回
// 完全相同的 *gorm.DB。
func NewDB(cfg *Config) (*gorm.DB, error) {
	return NewClient(cfg)
}

// NewDBWithLogger 是 NewClientWithLogger 的语义化别名。
func NewDBWithLogger(cfg *Config, log *zap.Logger) (*gorm.DB, error) {
	return NewClientWithLogger(cfg, log)
}

// newDialector 根据驱动类型构造 GORM dialector。
func newDialector(driver Driver, dsn string) gorm.Dialector {
	switch driver {
	case DriverMySQL:
		return mysql.Open(dsn)
	case DriverPostgres:
		return postgres.Open(dsn)
	case DriverSQLite:
		return sqlite.Open(dsn)
	case DriverSQLServer:
		return sqlserver.Open(dsn)
	default:
		return nil
	}
}

// buildGORMConfig 将配置文件字段映射为 gorm.Config。
func buildGORMConfig(cfg *Config, log *zap.Logger) *gorm.Config {
	opt := &gorm.Config{}

	if cfg.GORM != nil {
		opt.SkipDefaultTransaction = cfg.GORM.SkipDefaultTransaction
		opt.DryRun = cfg.GORM.DryRun
		opt.PrepareStmt = cfg.GORM.PrepareStmt
		opt.DisableNestedTransaction = cfg.GORM.DisableNestedTransaction
		opt.AllowGlobalUpdate = cfg.GORM.AllowGlobalUpdate
		opt.DisableAutomaticPing = cfg.GORM.DisableAutomaticPing
	}
	if cfg.Naming != nil {
		opt.NamingStrategy = schema.NamingStrategy{
			TablePrefix:         cfg.Naming.TablePrefix,
			SingularTable:       cfg.Naming.SingularTable,
			NoLowerCase:         cfg.Naming.NoLowerCase,
			IdentifierMaxLength: cfg.Naming.IdentifierMaxLength,
		}
	}
	if cfg.Logger != nil {
		opt.Logger = buildLogger(cfg.Logger, log)
	}
	if cfg.Migration != nil {
		opt.DisableForeignKeyConstraintWhenMigrating = cfg.Migration.DisableForeignKeyConstraintWhenMigrating
	}

	return opt
}

// buildLogger 构造基于 zap 的 GORM logger。
//
// log 为 nil 时使用 zap.NewNop()，避免因为未初始化全局 logger 产生隐式输出或 panic。
func buildLogger(cfg *LoggerConfig, log *zap.Logger) gormlogger.Interface {
	logConfig := gormlogger.Config{
		SlowThreshold:             cfg.SlowThreshold,
		Colorful:                  cfg.Colorful,
		IgnoreRecordNotFoundError: cfg.IgnoreRecordNotFoundError,
		ParameterizedQueries:      cfg.ParameterizedQueries,
		LogLevel:                  toGORMLogLevel(cfg.Level),
	}
	if logConfig.SlowThreshold == 0 {
		logConfig.SlowThreshold = 200 * time.Millisecond
	}
	if log == nil {
		log = zap.NewNop()
	}

	return zapGORMLogger{
		logger: log,
		config: logConfig,
		logSQL: cfg.LogSQL,
	}
}

// zapGORMLogger 使用 zap 承接 GORM logger.Interface。
//
// LogSQL=false 时普通 SQL Trace 不输出；错误和慢查询仍会输出耗时、影响行数和错误信息，
// 但 SQL 文本会被固定隐藏，避免敏感参数或完整 SQL 进入日志。
type zapGORMLogger struct {
	logger *zap.Logger
	config gormlogger.Config
	logSQL bool
}

func (l zapGORMLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	config := l.config
	config.LogLevel = level
	return zapGORMLogger{
		logger: l.logger,
		config: config,
		logSQL: l.logSQL,
	}
}

func (l zapGORMLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.config.LogLevel < gormlogger.Info {
		return
	}
	l.withContext(ctx).Info(fmt.Sprintf(msg, data...), zap.String("source", utils.FileWithLineNum()))
}

func (l zapGORMLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.config.LogLevel < gormlogger.Warn {
		return
	}
	l.withContext(ctx).Warn(fmt.Sprintf(msg, data...), zap.String("source", utils.FileWithLineNum()))
}

func (l zapGORMLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.config.LogLevel < gormlogger.Error {
		return
	}
	l.withContext(ctx).Error(fmt.Sprintf(msg, data...), zap.String("source", utils.FileWithLineNum()))
}

func (l zapGORMLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.config.LogLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	elapsedMS := float64(elapsed.Nanoseconds()) / 1e6

	switch {
	case err != nil && l.config.LogLevel >= gormlogger.Error && (!errors.Is(err, gormlogger.ErrRecordNotFound) || !l.config.IgnoreRecordNotFoundError):
		sql, rows := l.sqlAndRows(fc)
		l.withContext(ctx).Error("gorm sql error", l.traceFields(elapsed, elapsedMS, rows, sql, zap.Error(err))...)
	case l.config.SlowThreshold != 0 && elapsed > l.config.SlowThreshold && l.config.LogLevel >= gormlogger.Warn:
		sql, rows := l.sqlAndRows(fc)
		l.withContext(ctx).Warn("gorm slow sql", l.traceFields(elapsed, elapsedMS, rows, sql, zap.Duration("slow_threshold", l.config.SlowThreshold))...)
	case l.logSQL && l.config.LogLevel == gormlogger.Info:
		sql, rows := l.sqlAndRows(fc)
		l.withContext(ctx).Info("gorm sql", l.traceFields(elapsed, elapsedMS, rows, sql)...)
	}
}

func (l zapGORMLogger) ParamsFilter(_ context.Context, sql string, params ...interface{}) (string, []interface{}) {
	if l.config.ParameterizedQueries {
		return sql, nil
	}
	return sql, params
}

func (l zapGORMLogger) withContext(_ context.Context) *zap.Logger {
	if l.logger == nil {
		return zap.L()
	}
	return l.logger
}

func (l zapGORMLogger) sqlAndRows(fc func() (string, int64)) (string, int64) {
	sql, rows := fc()
	if !l.logSQL {
		sql = "[SQL hidden]"
	}
	return sql, rows
}

func (l zapGORMLogger) traceFields(elapsed time.Duration, elapsedMS float64, rows int64, sql string, extra ...zap.Field) []zap.Field {
	fields := []zap.Field{
		zap.String("source", utils.FileWithLineNum()),
		zap.Duration("elapsed", elapsed),
		zap.Float64("elapsed_ms", elapsedMS),
		zap.Int64("rows", rows),
		zap.String("sql", sql),
	}
	return append(fields, extra...)
}

func toGORMLogLevel(level LogLevel) gormlogger.LogLevel {
	switch level {
	case LogLevelSilent:
		return gormlogger.Silent
	case LogLevelError:
		return gormlogger.Error
	case LogLevelInfo:
		return gormlogger.Info
	case LogLevelWarn, "":
		return gormlogger.Warn
	default:
		return gormlogger.Warn
	}
}

// applyPoolConfig 设置底层 database/sql 连接池。
func applyPoolConfig(db *gorm.DB, cfg *PoolConfig) error {
	if cfg == nil {
		return nil
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("database get sql db failed: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	return nil
}

// applyResolverConfig 注册 GORM dbresolver 插件。
func applyResolverConfig(db *gorm.DB, cfg *Config) error {
	if cfg.Resolver == nil {
		return nil
	}

	resolver := dbresolver.Register(dbresolver.Config{
		Sources:           buildDialectors(cfg.Driver, cfg.Resolver.Sources),
		Replicas:          buildDialectors(cfg.Driver, cfg.Resolver.Replicas),
		Policy:            buildResolverPolicy(cfg.Resolver.Policy),
		TraceResolverMode: cfg.Resolver.TraceResolverMode,
	})

	if cfg.Pool != nil {
		resolver.
			SetMaxOpenConns(cfg.Pool.MaxOpenConns).
			SetMaxIdleConns(cfg.Pool.MaxIdleConns).
			SetConnMaxLifetime(cfg.Pool.ConnMaxLifetime).
			SetConnMaxIdleTime(cfg.Pool.ConnMaxIdleTime)
	}

	if err := db.Use(resolver); err != nil {
		return fmt.Errorf("database register dbresolver failed: %w", err)
	}
	return nil
}

func buildDialectors(driver Driver, dsns []string) []gorm.Dialector {
	if len(dsns) == 0 {
		return nil
	}

	dialectors := make([]gorm.Dialector, 0, len(dsns))
	for _, dsn := range dsns {
		dialectors = append(dialectors, newDialector(driver, dsn))
	}
	return dialectors
}

func buildResolverPolicy(policy ResolverPolicy) dbresolver.Policy {
	switch policy {
	case ResolverPolicyRandom, "":
		return dbresolver.RandomPolicy{}
	default:
		return dbresolver.RandomPolicy{}
	}
}

// instrumentClient 注册数据库链路追踪和指标采集插件。
func instrumentClient(db *gorm.DB, cfg *Config) error {
	if cfg.Monitoring == nil {
		return nil
	}

	if cfg.Monitoring.TracingEnabled {
		opts := []tracing.Option{
			tracing.WithDBSystem(string(cfg.Driver)),
			tracing.WithoutQueryVariables(),
		}
		if !cfg.Monitoring.MetricsEnabled {
			opts = append(opts, tracing.WithoutMetrics())
		}
		if err := registerTracingPlugin(db, opts...); err != nil {
			return err
		}
		return nil
	}

	if cfg.Monitoring.MetricsEnabled {
		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("database get sql db failed: %w", err)
		}
		if err := reportDBStatsMetrics(sqlDB); err != nil {
			return err
		}
	}
	return nil
}

func registerTracingPlugin(db *gorm.DB, opts ...tracing.Option) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("database register tracing plugin failed: %v", recovered)
		}
	}()
	if err := db.Use(tracing.NewPlugin(opts...)); err != nil {
		return fmt.Errorf("database register tracing plugin failed: %w", err)
	}
	return nil
}

func reportDBStatsMetrics(db *sql.DB) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("database register metrics failed: %v", recovered)
		}
	}()
	otelmetrics.ReportDBStatsMetrics(db)
	return nil
}

func closeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
