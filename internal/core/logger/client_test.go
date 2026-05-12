package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestNewWritesJSONFile(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "app.log")

	log, err := New(&Config{
		Level:           LevelInfo,
		Format:          FormatJSON,
		Output:          OutputFile,
		File:            logFile,
		AddCaller:       true,
		StacktraceLevel: StacktraceLevelNone,
		InitialFields: map[string]string{
			"service": "fox",
		},
		Rotation: &RotationConfig{
			MaxSize:    10,
			MaxBackups: 2,
			MaxAge:     7,
			Compress:   true,
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	log.Info("hello", zap.String("request_id", "req-1"))
	if err := log.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	content := readFile(t, logFile)
	wantContains := []string{
		`"level":"info"`,
		`"msg":"hello"`,
		`"service":"fox"`,
		`"request_id":"req-1"`,
	}
	for _, want := range wantContains {
		if !strings.Contains(content, want) {
			t.Fatalf("log content = %s, want to contain %q", content, want)
		}
	}
}

func TestNewAppliesEncoderConfig(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "app.log")

	log, err := New(&Config{
		Level:           LevelDebug,
		Format:          FormatJSON,
		Output:          OutputFile,
		File:            logFile,
		StacktraceLevel: StacktraceLevelNone,
		Encoder: &EncoderConfig{
			MessageKey:       "message",
			LevelKey:         "severity",
			TimeKey:          "time",
			StacktraceKey:    "stack",
			TimeEncoding:     "epoch",
			DurationEncoding: "millis",
			LevelEncoding:    "capital",
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	log.Debug("configured")
	if err := log.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	content := readFile(t, logFile)
	wantContains := []string{
		`"severity":"DEBUG"`,
		`"message":"configured"`,
		`"time":`,
	}
	for _, want := range wantContains {
		if !strings.Contains(content, want) {
			t.Fatalf("log content = %s, want to contain %q", content, want)
		}
	}
}

func TestNewFiltersByLevel(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "app.log")

	log, err := New(&Config{
		Level:           LevelWarn,
		Format:          FormatJSON,
		Output:          OutputFile,
		File:            logFile,
		StacktraceLevel: StacktraceLevelNone,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	log.Info("hidden")
	log.Warn("visible")
	if err := log.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	content := readFile(t, logFile)
	if strings.Contains(content, "hidden") {
		t.Fatalf("log content = %s, want info log filtered", content)
	}
	if !strings.Contains(content, "visible") {
		t.Fatalf("log content = %s, want warn log visible", content)
	}
}

func TestNewSugar(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "sugar.log")

	log, err := NewSugar(&Config{
		Output:          OutputFile,
		File:            logFile,
		StacktraceLevel: StacktraceLevelNone,
	})
	if err != nil {
		t.Fatalf("NewSugar() error = %v", err)
	}

	log.Infow("hello", "component", "test")
	if err := log.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	content := readFile(t, logFile)
	if !strings.Contains(content, `"component":"test"`) {
		t.Fatalf("log content = %s, want sugared field", content)
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	log, err := New(&Config{
		Output: OutputFile,
	})
	if err == nil {
		_ = log.Sync()
		t.Fatal("New() error = nil, want error")
	}
}

func TestNewRejectsNilConfig(t *testing.T) {
	log, err := New(nil)
	if err == nil {
		_ = log.Sync()
		t.Fatal("New() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "logger config is nil") {
		t.Fatalf("New() error = %v, want nil config error", err)
	}
}

func TestNewTrimsFilePath(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "trimmed.log")

	log, err := New(&Config{
		Output:          OutputFile,
		File:            " " + logFile + " ",
		StacktraceLevel: StacktraceLevelNone,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	log.Info("trimmed")
	if err := log.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	content := readFile(t, logFile)
	if !strings.Contains(content, "trimmed") {
		t.Fatalf("log content = %s, want trimmed file path to be used", content)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	return string(content)
}
