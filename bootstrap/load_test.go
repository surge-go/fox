package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	coreconfig "github.com/surge-go/fox/core/config"
	"github.com/surge-go/fox/core/logger"
	"github.com/surge-go/fox/core/server"
)

func TestLoadConfig(t *testing.T) {
	path := writeBootstrapConfig(t, t.TempDir(), "config.yaml", `
logger:
  level: info
  output: stdout
server:
  mode: test
  addr: ":0"
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Logger == nil {
		t.Fatal("Logger config should not be nil")
	}
	if cfg.Logger.Level != logger.LevelInfo {
		t.Fatalf("Logger.Level = %q, want %q", cfg.Logger.Level, logger.LevelInfo)
	}
	if cfg.Server == nil {
		t.Fatal("Server config should not be nil")
	}
	if cfg.Server.Mode != server.ModeTest {
		t.Fatalf("Server.Mode = %q, want %q", cfg.Server.Mode, server.ModeTest)
	}
	if cfg.Server.Addr != ":0" {
		t.Fatalf("Server.Addr = %q, want %q", cfg.Server.Addr, ":0")
	}
}

func TestLoadConfigRejectsEmptyPath(t *testing.T) {
	if _, err := LoadConfig(" "); err == nil {
		t.Fatal("LoadConfig() should reject empty path")
	}
}

func TestLoadConfigWithOptions(t *testing.T) {
	dir := t.TempDir()
	writeBootstrapConfig(t, dir, "fox.yaml", `
logger:
  level: warn
  output: stderr
`)

	cfg, err := LoadConfigWithOptions(
		coreconfig.WithConfigName("fox"),
		coreconfig.WithConfigType("yaml"),
		coreconfig.WithConfigPaths(dir),
	)
	if err != nil {
		t.Fatalf("LoadConfigWithOptions() error = %v", err)
	}
	if cfg.Logger == nil {
		t.Fatal("Logger config should not be nil")
	}
	if cfg.Logger.Level != logger.LevelWarn {
		t.Fatalf("Logger.Level = %q, want %q", cfg.Logger.Level, logger.LevelWarn)
	}
}

func TestLoadConfigOptionsCanOverridePath(t *testing.T) {
	dir := t.TempDir()
	defaultPath := writeBootstrapConfig(t, dir, "default.yaml", `
logger:
  level: info
  output: stdout
`)
	overridePath := writeBootstrapConfig(t, dir, "override.yaml", `
logger:
  level: error
  output: stderr
`)

	cfg, err := LoadConfig(defaultPath, coreconfig.WithConfigFile(overridePath))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Logger == nil {
		t.Fatal("Logger config should not be nil")
	}
	if cfg.Logger.Level != logger.LevelError {
		t.Fatalf("Logger.Level = %q, want %q", cfg.Logger.Level, logger.LevelError)
	}
}

func writeBootstrapConfig(t *testing.T, dir, filename, content string) string {
	t.Helper()

	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
