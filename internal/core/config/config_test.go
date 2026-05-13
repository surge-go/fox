package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testYAML = `
app:
  name: test-app
  port: 8080
  debug: true
  timeout: 5s
  tags:
    - web
    - api
database:
  host: localhost
  port: 5432
  name: testdb
`

func writeTestConfig(t *testing.T, dir, filename, content string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
	return path
}

func TestNew(t *testing.T) {
	c := New()
	assert.NotNil(t, c)
	assert.NotNil(t, c.viper)
	assert.False(t, c.protected)
	assert.False(t, c.autoWatch)
}

func TestNewWithOptions(t *testing.T) {
	c := New(
		WithProtected(true),
		WithAutoWatch(true),
		WithEnvPrefix("TEST"),
	)
	assert.NotNil(t, c)
	assert.True(t, c.protected)
	assert.True(t, c.autoWatch)
	assert.Equal(t, "TEST", c.envPrefix)
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	c := New(WithConfigFile(cfgPath))
	err := c.Load()
	require.NoError(t, err)

	assert.Equal(t, "test-app", c.GetString("app.name"))
	assert.Equal(t, 8080, c.GetInt("app.port"))
}

func TestLoadWithNameAndPaths(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, "myconfig.yaml", testYAML)

	c := New(
		WithConfigName("myconfig"),
		WithConfigType("yaml"),
		WithConfigPaths(dir),
	)
	err := c.Load()
	require.NoError(t, err)

	assert.Equal(t, "test-app", c.GetString("app.name"))
}

func TestGetString(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	c := New(WithConfigFile(cfgPath))
	require.NoError(t, c.Load())

	assert.Equal(t, "test-app", c.GetString("app.name"))
	assert.Equal(t, "", c.GetString("nonexistent"))
}

func TestGetInt(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	c := New(WithConfigFile(cfgPath))
	require.NoError(t, c.Load())

	assert.Equal(t, 8080, c.GetInt("app.port"))
	assert.Equal(t, int64(5432), c.GetInt64("database.port"))
	assert.Equal(t, 0, c.GetInt("nonexistent"))
}

func TestGetBool(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	c := New(WithConfigFile(cfgPath))
	require.NoError(t, c.Load())

	assert.True(t, c.GetBool("app.debug"))
	assert.False(t, c.GetBool("nonexistent"))
}

func TestGetDuration(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	c := New(WithConfigFile(cfgPath))
	require.NoError(t, c.Load())

	assert.Equal(t, 5*time.Second, c.GetDuration("app.timeout"))
}

func TestGetStringSlice(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	c := New(WithConfigFile(cfgPath))
	require.NoError(t, c.Load())

	tags := c.GetStringSlice("app.tags")
	assert.Equal(t, []string{"web", "api"}, tags)
}

func TestGenericGet(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	c := New(WithConfigFile(cfgPath))
	require.NoError(t, c.Load())

	name := Get[string](c, "app.name")
	assert.Equal(t, "test-app", name)

	debug := Get[bool](c, "app.debug")
	assert.True(t, debug)

	// nonexistent key returns zero value
	missing := Get[string](c, "nonexistent")
	assert.Equal(t, "", missing)
}

func TestSet(t *testing.T) {
	c := New()
	c.Set("foo", "bar")
	assert.Equal(t, "bar", c.GetString("foo"))

	c.Set("count", 42)
	assert.Equal(t, 42, c.GetInt("count"))
}

func TestIsSet(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	c := New(WithConfigFile(cfgPath))
	require.NoError(t, c.Load())

	assert.True(t, c.IsSet("app.name"))
	assert.True(t, c.IsSet("database.host"))
	assert.False(t, c.IsSet("nonexistent.key"))
}

func TestAllSettings(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	c := New(WithConfigFile(cfgPath))
	require.NoError(t, c.Load())

	all := c.AllSettings()
	assert.NotNil(t, all)
	assert.Contains(t, all, "app")
	assert.Contains(t, all, "database")
}

func TestSub(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	c := New(WithConfigFile(cfgPath))
	require.NoError(t, c.Load())

	dbCfg := c.Sub("database")
	require.NotNil(t, dbCfg)
	assert.Equal(t, "localhost", dbCfg.GetString("host"))
	assert.Equal(t, 5432, dbCfg.GetInt("port"))
	assert.Equal(t, "testdb", dbCfg.GetString("name"))

	// nonexistent sub returns nil
	nilSub := c.Sub("nonexistent")
	assert.Nil(t, nilSub)
}

func TestUnmarshal(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	c := New(WithConfigFile(cfgPath))
	require.NoError(t, c.Load())

	var cfg struct {
		App struct {
			Name    string        `mapstructure:"name"`
			Port    int           `mapstructure:"port"`
			Debug   bool          `mapstructure:"debug"`
			Timeout time.Duration `mapstructure:"timeout"`
		} `mapstructure:"app"`
		Database struct {
			Host string `mapstructure:"host"`
			Port int    `mapstructure:"port"`
			Name string `mapstructure:"name"`
		} `mapstructure:"database"`
	}

	err := c.Unmarshal(&cfg)
	require.NoError(t, err)
	assert.Equal(t, "test-app", cfg.App.Name)
	assert.Equal(t, 8080, cfg.App.Port)
	assert.True(t, cfg.App.Debug)
	assert.Equal(t, "localhost", cfg.Database.Host)
}

func TestUnmarshalKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	c := New(WithConfigFile(cfgPath))
	require.NoError(t, c.Load())

	var db struct {
		Host string `mapstructure:"host"`
		Port int    `mapstructure:"port"`
		Name string `mapstructure:"name"`
	}

	err := c.UnmarshalKey("database", &db)
	require.NoError(t, err)
	assert.Equal(t, "localhost", db.Host)
	assert.Equal(t, 5432, db.Port)
	assert.Equal(t, "testdb", db.Name)
}

func TestDefault(t *testing.T) {
	// Reset global state for test isolation
	defaultMu.Lock()
	old := defaultInstance
	defaultInstance = nil
	defaultMu.Unlock()
	t.Cleanup(func() {
		defaultMu.Lock()
		defaultInstance = old
		defaultMu.Unlock()
	})

	d := Default()
	assert.NotNil(t, d)

	// Calling Default() again returns the same instance
	d2 := Default()
	assert.Same(t, d, d2)
}

func TestSetDefault(t *testing.T) {
	// Reset global state
	defaultMu.Lock()
	old := defaultInstance
	defaultInstance = nil
	defaultMu.Unlock()
	t.Cleanup(func() {
		defaultMu.Lock()
		defaultInstance = old
		defaultMu.Unlock()
	})

	c := New()
	SetDefault(c)
	assert.Same(t, c, Default())
}

func TestWithDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", `
app:
  name: myapp
`)

	defaults := map[string]any{
		"app.port":  3000,
		"app.debug": false,
	}

	c := New(
		WithConfigFile(cfgPath),
		WithDefaults(defaults),
	)
	require.NoError(t, c.Load())

	// Explicit value overrides default
	assert.Equal(t, "myapp", c.GetString("app.name"))
	// Default values are used when not in config
	assert.Equal(t, 3000, c.GetInt("app.port"))
	assert.False(t, c.GetBool("app.debug"))
}

func TestWithEnvPrefix(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", `
app:
  name: myapp
`)

	t.Setenv("MYAPP_APP_NAME", "env-app")

	c := New(
		WithConfigFile(cfgPath),
		WithEnvPrefix("MYAPP"),
		WithEnvKeyReplacer(strings.NewReplacer(".", "_")),
	)
	require.NoError(t, c.Load())

	// Environment variable should override config file value
	assert.Equal(t, "env-app", c.GetString("app.name"))
}

func TestWithOnError(t *testing.T) {
	var gotErr error
	c := New(
		WithOnError(func(err error) {
			gotErr = err
		}),
	)
	assert.NotNil(t, c)
	assert.NotNil(t, c.onError)
	// onError is tested indirectly through protected mode restore failures
	_ = gotErr
}

func TestProtectedMode(t *testing.T) {
	dir := t.TempDir()
	originalContent := `
app:
  name: original
`
	cfgPath := writeTestConfig(t, dir, "config.yaml", originalContent)

	c := New(
		WithConfigFile(cfgPath),
		WithProtected(true),
		WithAutoWatch(true),
	)
	require.NoError(t, c.Load())
	assert.True(t, c.IsProtected())

	// Modify the file externally
	modifiedContent := `
app:
  name: modified
`
	err := os.WriteFile(cfgPath, []byte(modifiedContent), 0644)
	require.NoError(t, err)

	// Wait for fsnotify to detect the change and restore
	time.Sleep(500 * time.Millisecond)

	// File should be restored to original content
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, originalContent, string(data))
}

func TestNonProtectedMode(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	changed := make(chan struct{}, 1)
	c := New(
		WithConfigFile(cfgPath),
		WithProtected(false),
		WithAutoWatch(true),
		WithOnChange(func() {
			select {
			case changed <- struct{}{}:
			default:
			}
		}),
	)
	require.NoError(t, c.Load())
	assert.False(t, c.IsProtected())

	// Modify the file
	newContent := `
app:
  name: updated-app
  port: 9090
  debug: false
  timeout: 10s
  tags:
    - web
database:
  host: remotehost
  port: 5432
  name: testdb
`
	err := os.WriteFile(cfgPath, []byte(newContent), 0644)
	require.NoError(t, err)

	// Wait for onChange callback
	select {
	case <-changed:
		// callback was triggered
	case <-time.After(2 * time.Second):
		t.Fatal("onChange callback was not triggered within timeout")
	}
}

func TestSetProtected(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	c := New(
		WithConfigFile(cfgPath),
		WithProtected(false),
		WithAutoWatch(true),
	)
	require.NoError(t, c.Load())
	assert.False(t, c.IsProtected())

	// Dynamically enable protection
	c.SetProtected(true)
	assert.True(t, c.IsProtected())

	// Dynamically disable protection
	c.SetProtected(false)
	assert.False(t, c.IsProtected())
}

func TestConfigFileNotFound(t *testing.T) {
	c := New(WithConfigFile("/nonexistent/path/config.yaml"))
	err := c.Load()
	assert.Error(t, err)
}

func TestConfigFileNotFoundByName(t *testing.T) {
	dir := t.TempDir()
	c := New(
		WithConfigName("nonexistent"),
		WithConfigType("yaml"),
		WithConfigPaths(dir),
	)
	err := c.Load()
	assert.Error(t, err)
}

func TestConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	c := New(WithConfigFile(cfgPath))
	require.NoError(t, c.Load())

	var wg sync.WaitGroup
	const goroutines = 50

	// Concurrent reads
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.GetString("app.name")
			_ = c.GetInt("app.port")
			_ = c.GetBool("app.debug")
			_ = c.IsSet("app.name")
			_ = c.AllSettings()
		}()
	}

	// Concurrent writes
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c.Set("dynamic.key", i)
		}(i)
	}

	wg.Wait()

	// Verify config is still readable after concurrent access
	assert.Equal(t, "test-app", c.GetString("app.name"))
}

func TestStartStopWatch(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	c := New(WithConfigFile(cfgPath))
	require.NoError(t, c.Load())

	// Start watching
	err := c.StartWatch()
	assert.NoError(t, err)

	// Starting again should be a no-op
	err = c.StartWatch()
	assert.NoError(t, err)

	// Stop watching
	c.StopWatch()
	c.Close()
}

func TestGetStringMap(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, "config.yaml", testYAML)

	c := New(WithConfigFile(cfgPath))
	require.NoError(t, c.Load())

	m := c.GetStringMap("database")
	assert.NotNil(t, m)
	assert.Equal(t, "localhost", m["host"])
}

func TestGetStringMapString(t *testing.T) {
	dir := t.TempDir()
	content := `
labels:
  env: production
  team: backend
`
	cfgPath := writeTestConfig(t, dir, "config.yaml", content)

	c := New(WithConfigFile(cfgPath))
	require.NoError(t, c.Load())

	m := c.GetStringMapString("labels")
	assert.Equal(t, "production", m["env"])
	assert.Equal(t, "backend", m["team"])
}

func TestGetFloat64(t *testing.T) {
	dir := t.TempDir()
	content := `
metrics:
  rate: 0.95
`
	cfgPath := writeTestConfig(t, dir, "config.yaml", content)

	c := New(WithConfigFile(cfgPath))
	require.NoError(t, c.Load())

	assert.InDelta(t, 0.95, c.GetFloat64("metrics.rate"), 0.001)
}

func TestViper(t *testing.T) {
	c := New()
	assert.NotNil(t, c.Viper())
}
