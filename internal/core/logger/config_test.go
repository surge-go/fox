package logger

import (
	"strings"
	"testing"
)

func TestConfigValidateDefaults(t *testing.T) {
	cfg := &Config{}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidate(t *testing.T) {
	cfg := &Config{
		Level:           LevelInfo,
		Format:          FormatJSON,
		Output:          OutputFile,
		File:            "/var/log/fox/app.log",
		ErrorOutput:     "stderr",
		AddCaller:       true,
		StacktraceLevel: StacktraceLevelError,
		InitialFields: map[string]string{
			"service": "fox",
			"env":     "prod",
		},
		Encoder: &EncoderConfig{
			MessageKey:       "msg",
			LevelKey:         "level",
			TimeKey:          "ts",
			CallerKey:        "caller",
			StacktraceKey:    "stacktrace",
			TimeEncoding:     "iso8601",
			DurationEncoding: "seconds",
			LevelEncoding:    "lowercase",
		},
		Rotation: &RotationConfig{
			MaxSize:    100,
			MaxAge:     30,
			MaxBackups: 10,
			LocalTime:  true,
			Compress:   true,
		},
		Sampling: &SamplingConfig{
			Enabled:    true,
			Initial:    100,
			Thereafter: 100,
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateInvalidConfig(t *testing.T) {
	cfg := &Config{
		Level:           Level("trace"),
		Format:          Format("text"),
		Output:          Output("network"),
		File:            "/var/log/fox/app.log",
		ErrorOutput:     " ",
		CallerSkip:      -1,
		StacktraceLevel: StacktraceLevel("warn"),
		InitialFields: map[string]string{
			"": "empty",
		},
		Encoder: &EncoderConfig{
			TimeEncoding:     "rfc3339",
			DurationEncoding: "duration",
			LevelEncoding:    "upper",
		},
		Rotation: &RotationConfig{
			MaxSize:    -1,
			MaxAge:     -1,
			MaxBackups: -1,
		},
		Sampling: &SamplingConfig{
			Enabled:    true,
			Initial:    0,
			Thereafter: 0,
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}

	wantContains := []string{
		"logger level must be one of",
		"logger format must be one of",
		"logger output must be one of",
		"logger file requires output to be file",
		"logger error_output must not be blank",
		"logger caller_skip must be greater than or equal to 0",
		"logger stacktrace_level must be one of",
		"logger initial_fields key must not be empty",
		"logger encoder.time_encoding must be one of",
		"logger encoder.duration_encoding must be one of",
		"logger encoder.level_encoding must be one of",
		"logger rotation requires output to be file",
		"logger rotation.max_size must be greater than or equal to 0",
		"logger rotation.max_age must be greater than or equal to 0",
		"logger rotation.max_backups must be greater than or equal to 0",
		"logger sampling.initial must be greater than 0 when sampling is enabled",
		"logger sampling.thereafter must be greater than 0 when sampling is enabled",
	}
	for _, want := range wantContains {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Validate() error = %v, want to contain %q", err, want)
		}
	}
}

func TestConfigValidateFileOutputRequiresFile(t *testing.T) {
	cfg := &Config{
		Output: OutputFile,
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "logger file must not be empty when output is file") {
		t.Fatalf("Validate() error = %v, want file required error", err)
	}
}
