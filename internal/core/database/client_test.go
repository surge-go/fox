package database

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestNewClientSQLite(t *testing.T) {
	db, err := NewClient(&Config{
		Driver: DriverSQLite,
		DSN:    sqliteMemoryDSN(t, "primary"),
		GORM: &GORMConfig{
			DisableAutomaticPing: true,
		},
		Pool: &PoolConfig{
			MaxOpenConns: 1,
			MaxIdleConns: 1,
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer closeTestDB(t, db)

	var _ *gorm.DB = db
	if err := db.Exec("CREATE TABLE users (id integer primary key, name text)").Error; err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
}

func TestNewDBAlias(t *testing.T) {
	db, err := NewDB(&Config{
		Driver: DriverSQLite,
		DSN:    sqliteMemoryDSN(t, "alias"),
		GORM: &GORMConfig{
			DisableAutomaticPing: true,
		},
	})
	if err != nil {
		t.Fatalf("NewDB() error = %v", err)
	}
	defer closeTestDB(t, db)

	var _ *gorm.DB = db
}

func TestNewClientWithLoggerUsesProvidedZapLogger(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	db, err := NewClientWithLogger(&Config{
		Driver: DriverSQLite,
		DSN:    sqliteMemoryDSN(t, "zap_logger"),
		GORM: &GORMConfig{
			DisableAutomaticPing: true,
		},
		Logger: &LoggerConfig{
			Level:  LogLevelInfo,
			LogSQL: true,
		},
	}, zap.New(core))
	if err != nil {
		t.Fatalf("NewClientWithLogger() error = %v", err)
	}
	defer closeTestDB(t, db)

	if err := db.Exec("CREATE TABLE users (id integer primary key, name text)").Error; err != nil {
		t.Fatalf("Exec() error = %v", err)
	}

	entries := logs.FilterMessage("gorm sql").All()
	if len(entries) == 0 {
		t.Fatal("provided zap logger did not receive gorm sql log")
	}
	sqlValue, ok := entries[0].ContextMap()["sql"].(string)
	if !ok || !strings.Contains(sqlValue, "CREATE TABLE users") {
		t.Fatalf("gorm sql log sql = %v, want create table statement", entries[0].ContextMap()["sql"])
	}
}

func TestNewClientAppliesConfig(t *testing.T) {
	db, err := NewClient(&Config{
		Driver: DriverSQLite,
		DSN:    sqliteMemoryDSN(t, "config"),
		GORM: &GORMConfig{
			SkipDefaultTransaction:   true,
			PrepareStmt:              true,
			DisableNestedTransaction: true,
			AllowGlobalUpdate:        true,
			DisableAutomaticPing:     true,
		},
		Naming: &NamingConfig{
			TablePrefix:         "t_",
			SingularTable:       true,
			NoLowerCase:         true,
			IdentifierMaxLength: 32,
		},
		Logger: &LoggerConfig{
			Level:                LogLevelInfo,
			LogSQL:               false,
			ParameterizedQueries: true,
		},
		Migration: &MigrationConfig{
			DisableForeignKeyConstraintWhenMigrating: true,
		},
		Pool: &PoolConfig{
			MaxOpenConns:    3,
			MaxIdleConns:    2,
			ConnMaxLifetime: time.Minute,
			ConnMaxIdleTime: time.Second,
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer closeTestDB(t, db)

	if !db.Config.SkipDefaultTransaction {
		t.Fatal("SkipDefaultTransaction = false, want true")
	}
	if !db.Config.PrepareStmt {
		t.Fatal("PrepareStmt = false, want true")
	}
	if !db.Config.DisableNestedTransaction {
		t.Fatal("DisableNestedTransaction = false, want true")
	}
	if !db.Config.AllowGlobalUpdate {
		t.Fatal("AllowGlobalUpdate = false, want true")
	}
	if !db.Config.DisableAutomaticPing {
		t.Fatal("DisableAutomaticPing = false, want true")
	}
	if !db.Config.DisableForeignKeyConstraintWhenMigrating {
		t.Fatal("DisableForeignKeyConstraintWhenMigrating = false, want true")
	}

	naming, ok := db.Config.NamingStrategy.(schema.NamingStrategy)
	if !ok {
		t.Fatalf("NamingStrategy type = %T, want schema.NamingStrategy", db.Config.NamingStrategy)
	}
	if naming.TablePrefix != "t_" || !naming.SingularTable || !naming.NoLowerCase || naming.IdentifierMaxLength != 32 {
		t.Fatalf("NamingStrategy = %+v, want configured values", naming)
	}

	if _, ok := db.Config.Logger.(zapGORMLogger); !ok {
		t.Fatalf("Logger type = %T, want zapGORMLogger", db.Config.Logger)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("DB() error = %v", err)
	}
	if got := sqlDB.Stats().MaxOpenConnections; got != 3 {
		t.Fatalf("MaxOpenConnections = %d, want 3", got)
	}
}

func TestSQLHiddenLoggerKeepsSlowAndErrorSignals(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	log := zapGORMLogger{
		logger: zap.New(core),
		config: gormlogger.Config{
			SlowThreshold: time.Millisecond,
			LogLevel:      gormlogger.Info,
		},
		logSQL: false,
	}

	log.Trace(context.Background(), time.Now(), func() (string, int64) {
		return "select * from users where phone = '13800000000'", 1
	}, nil)
	if logs.Len() != 0 {
		t.Fatalf("normal trace log len = %d, want 0", logs.Len())
	}

	log.Trace(context.Background(), time.Now().Add(-2*time.Millisecond), func() (string, int64) {
		return "select * from users where phone = '13800000000'", 1
	}, nil)
	slowEntry := logs.All()[0]
	if slowEntry.Message != "gorm slow sql" {
		t.Fatalf("slow trace message = %q, want gorm slow sql", slowEntry.Message)
	}
	if got := slowEntry.ContextMap()["sql"]; got != "[SQL hidden]" {
		t.Fatalf("slow trace sql = %v, want hidden sql", got)
	}

	logs.TakeAll()
	log.Trace(context.Background(), time.Now(), func() (string, int64) {
		return "select * from users where token = 'secret'", -1
	}, errors.New("query failed"))
	errorEntry := logs.All()[0]
	if errorEntry.Message != "gorm sql error" {
		t.Fatalf("error trace message = %q, want gorm sql error", errorEntry.Message)
	}
	if got := errorEntry.ContextMap()["sql"]; got != "[SQL hidden]" {
		t.Fatalf("error trace sql = %v, want hidden sql", got)
	}
	if got := errorEntry.ContextMap()["error"]; got != "query failed" {
		t.Fatalf("error trace error = %v, want query failed", got)
	}
}

func TestZapGORMLoggerParamsFilter(t *testing.T) {
	log := zapGORMLogger{
		config: gormlogger.Config{
			ParameterizedQueries: true,
		},
	}

	sql, params := log.ParamsFilter(context.Background(), "select * from users where id = ?", 1)
	if sql != "select * from users where id = ?" {
		t.Fatalf("ParamsFilter() sql = %q, want original sql", sql)
	}
	if params != nil {
		t.Fatalf("ParamsFilter() params = %v, want nil", params)
	}

	log.config.ParameterizedQueries = false
	_, params = log.ParamsFilter(context.Background(), "select * from users where id = ?", 1)
	if len(params) != 1 || params[0] != 1 {
		t.Fatalf("ParamsFilter() params = %v, want original params", params)
	}
}

func TestNewClientRegistersResolver(t *testing.T) {
	db, err := NewClient(&Config{
		Driver: DriverSQLite,
		DSN:    sqliteMemoryDSN(t, "resolver_primary"),
		GORM: &GORMConfig{
			DisableAutomaticPing: true,
		},
		Resolver: &ResolverConfig{
			Replicas: []string{sqliteMemoryDSN(t, "resolver_replica")},
			Policy:   ResolverPolicyRandom,
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer closeTestDB(t, db)

	if _, ok := db.Plugins["gorm:db_resolver"]; !ok {
		t.Fatal("dbresolver plugin is not registered")
	}
}

func TestNewClientRegistersTracing(t *testing.T) {
	db, err := NewClient(&Config{
		Driver: DriverSQLite,
		DSN:    sqliteMemoryDSN(t, "tracing"),
		GORM: &GORMConfig{
			DisableAutomaticPing: true,
		},
		Monitoring: &MonitoringConfig{
			TracingEnabled: true,
			MetricsEnabled: false,
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer closeTestDB(t, db)

	if _, ok := db.Plugins["otelgorm"]; !ok {
		t.Fatal("opentelemetry tracing plugin is not registered")
	}
}

func TestNewClientRejectsInvalidConfig(t *testing.T) {
	db, err := NewClient(&Config{})
	if err == nil {
		closeTestDB(t, db)
		t.Fatal("NewClient() error = nil, want error")
	}
}

func sqliteMemoryDSN(t *testing.T, name string) string {
	t.Helper()
	testName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	return "file:" + testName + "_" + name + "?mode=memory&cache=shared"
}

func closeTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("DB() error = %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
