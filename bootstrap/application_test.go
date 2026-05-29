package bootstrap

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/surge-go/fox/core/logger"
	"github.com/surge-go/fox/core/server"
)

func TestNewNilConfig(t *testing.T) {
	_, err := New(nil)
	if err == nil {
		t.Fatal("New(nil) should fail")
	}
}

func TestNewEmptyConfig(t *testing.T) {
	app, err := New(&Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if app == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNewWithLogger(t *testing.T) {
	app, err := New(&Config{
		Logger: &logger.Config{
			Level:  "info",
			Output: logger.OutputStdout,
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if app.Logger() == nil {
		t.Fatal("Logger() returned nil")
	}
}

func TestNewStoresConfigCopy(t *testing.T) {
	cfg := &Config{
		Logger: &logger.Config{
			Level:  "info",
			Output: logger.OutputStdout,
		},
	}

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cfg.Logger = nil
	if app.cfg == cfg {
		t.Fatal("Application should not keep caller config pointer")
	}
	if app.cfg.Logger == nil {
		t.Fatal("Application config copy should keep original logger config")
	}
}

func TestNewWithServer(t *testing.T) {
	app, err := New(&Config{
		Server: &server.Config{
			Mode: server.ModeTest,
			Addr: ":0",
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if app.Server() == nil {
		t.Fatal("Server() returned nil")
	}
}

func TestAccessorsReturnNilWhenUnconfigured(t *testing.T) {
	app, err := New(&Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if app.Server() != nil {
		t.Fatal("Server() should be nil")
	}
	if app.DB() != nil {
		t.Fatal("DB() should be nil")
	}
	if app.Redis() != nil {
		t.Fatal("Redis() should be nil")
	}
	if app.Tracing() != nil {
		t.Fatal("Tracing() should be nil")
	}
	if app.Metrics() != nil {
		t.Fatal("Metrics() should be nil")
	}
}

func TestLoggerFallback(t *testing.T) {
	app, err := New(&Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	l := app.Logger()
	if l == nil {
		t.Fatal("Logger() returned nil")
	}
	if l != zap.L() {
		t.Fatal("Logger() should return zap.L() when not configured")
	}
}

func TestOnStartHooks(t *testing.T) {
	app, err := New(&Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var order []int
	app.OnStart(func(ctx context.Context) error {
		order = append(order, 1)
		return nil
	})
	app.OnStart(func(ctx context.Context) error {
		order = append(order, 2)
		return nil
	})

	if err := app.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("hooks executed in wrong order: %v", order)
	}
}

func TestOnStartHookError(t *testing.T) {
	app, err := New(&Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	app.OnStart(func(ctx context.Context) error {
		return errors.New("hook failed")
	})

	err = app.Run()
	if err == nil {
		t.Fatal("Run() should fail when start hook fails")
	}
	if err.Error() != "bootstrap: start hook failed: hook failed" {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestInitErrorIncludesCleanupError(t *testing.T) {
	app := &Application{}
	app.OnStop(func(ctx context.Context) error {
		return errors.New("cleanup failed")
	})

	err := app.initError("database", errors.New("init failed"))
	if err == nil {
		t.Fatal("initError() should return error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "bootstrap: init database: init failed") {
		t.Fatalf("initError() missing init error: %v", err)
	}
	if !strings.Contains(msg, "bootstrap: stop hook failed: cleanup failed") {
		t.Fatalf("initError() missing cleanup error: %v", err)
	}
}

func TestNilHooksAreIgnored(t *testing.T) {
	app, err := New(&Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	app.OnStart(nil)
	app.OnStop(nil)

	if err := app.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestOnStopHooksReverseOrder(t *testing.T) {
	app, err := New(&Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var order []int
	app.OnStop(func(ctx context.Context) error {
		order = append(order, 1)
		return nil
	})
	app.OnStop(func(ctx context.Context) error {
		order = append(order, 2)
		return nil
	})

	if err := app.shutdown(); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}

	if len(order) != 2 || order[0] != 2 || order[1] != 1 {
		t.Fatalf("stop hooks should execute in reverse order: %v", order)
	}
}

func TestShutdownReturnsStopHookErrors(t *testing.T) {
	app, err := New(&Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	app.OnStop(func(ctx context.Context) error {
		return errors.New("first")
	})
	app.OnStop(func(ctx context.Context) error {
		return errors.New("second")
	})

	err = app.Shutdown(context.Background())
	if err == nil {
		t.Fatal("Shutdown() should return stop hook errors")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bootstrap: stop hook failed: second") {
		t.Fatalf("Shutdown() error missing second hook: %v", err)
	}
	if !strings.Contains(msg, "bootstrap: stop hook failed: first") {
		t.Fatalf("Shutdown() error missing first hook: %v", err)
	}
}

func TestShutdownIdempotent(t *testing.T) {
	app, err := New(&Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var count atomic.Int32
	app.OnStop(func(ctx context.Context) error {
		count.Add(1)
		return nil
	})

	// 多次调用，stop hook 只应执行一次
	if err := app.shutdown(); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
	if err := app.shutdown(); err != nil {
		t.Fatalf("shutdown() second call error = %v", err)
	}
	if err := app.shutdown(); err != nil {
		t.Fatalf("shutdown() third call error = %v", err)
	}

	if n := count.Load(); n != 1 {
		t.Fatalf("stop hook called %d times, want 1", n)
	}
}

func TestShutdownIdempotentReturnsStoredError(t *testing.T) {
	app, err := New(&Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	app.OnStop(func(ctx context.Context) error {
		return errors.New("cleanup failed")
	})

	first := app.Shutdown(context.Background())
	second := app.Shutdown(context.Background())
	if first == nil {
		t.Fatal("first Shutdown() should return error")
	}
	if second != first {
		t.Fatalf("second Shutdown() should return stored error, got %v want %v", second, first)
	}
}

func TestShutdownMethodIdempotent(t *testing.T) {
	app, err := New(&Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var count atomic.Int32
	app.OnStop(func(ctx context.Context) error {
		count.Add(1)
		return nil
	})

	// Shutdown 和 shutdown 都走同一个 once
	app.Shutdown(context.Background())
	app.Shutdown(context.Background())

	if n := count.Load(); n != 1 {
		t.Fatalf("stop hook called %d times, want 1", n)
	}
}

func TestRunWithServerAndHooks(t *testing.T) {
	app, err := New(&Config{
		Server: &server.Config{
			Mode: server.ModeTest,
			Addr: ":0",
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var started atomic.Bool
	app.OnStart(func(ctx context.Context) error {
		started.Store(true)
		return nil
	})

	go func() {
		time.Sleep(100 * time.Millisecond)
		app.Shutdown(context.Background())
	}()

	if err := app.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !started.Load() {
		t.Fatal("start hook was not executed")
	}
}

func TestNewWithLoggerDoesNotRequireCleanupOnFailure(t *testing.T) {
	app, err := New(&Config{
		Logger: &logger.Config{
			Level:  "info",
			Output: logger.OutputStdout,
		},
		Tracing: nil,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if app.Logger() == nil {
		t.Fatal("Logger() should not be nil")
	}
}
