package redis

import (
	"strings"
	"testing"
	"time"
)

func TestConfigValidateStandalone(t *testing.T) {
	cfg := &Config{
		Addrs: []string{"127.0.0.1:6379"},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateSentinelRequiresMasterName(t *testing.T) {
	cfg := &Config{
		Mode:  ModeSentinel,
		Addrs: []string{"127.0.0.1:26379"},
		Sentinel: &SentinelConfig{
			MasterName: " ",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "redis sentinel.master_name must not be empty") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateClusterRejectsDB(t *testing.T) {
	cfg := &Config{
		Mode:    ModeCluster,
		Addrs:   []string{"127.0.0.1:6379"},
		DB:      1,
		Cluster: &ClusterConfig{},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "redis cluster mode requires db to be 0") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateClusterAllowsDisableRedirects(t *testing.T) {
	cfg := &Config{
		Mode:  ModeCluster,
		Addrs: []string{"127.0.0.1:6379"},
		Cluster: &ClusterConfig{
			MaxRedirects: -1,
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateSubConfigs(t *testing.T) {
	cfg := &Config{
		Addrs: []string{"127.0.0.1:6379"},
		Timeout: &TimeoutConfig{
			DialTimeout: -time.Second,
		},
		Retry: &RetryConfig{
			MinRetryBackoff: 2 * time.Second,
			MaxRetryBackoff: time.Second,
		},
		Pool: &PoolConfig{
			MinIdleConns:          2,
			MaxIdleConns:          1,
			ConnMaxLifetimeJitter: time.Second,
		},
		Buffer: &BufferConfig{
			ReadBufferSize: -1,
		},
		TLS: &TLSConfig{
			Enabled:  true,
			CertFile: "client.crt",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}

	wantContains := []string{
		"redis timeout.dial_timeout must be greater than or equal to 0",
		"redis retry.min_retry_backoff must be less than or equal to max_retry_backoff",
		"redis pool.min_idle_conns must be less than or equal to max_idle_conns",
		"redis pool.conn_max_lifetime_jitter requires conn_max_lifetime",
		"redis buffer.read_buffer_size must be greater than or equal to 0",
		"redis tls.cert_file and tls.key_file must be set together",
	}
	for _, want := range wantContains {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Validate() error = %v, want to contain %q", err, want)
		}
	}
}

func TestConfigValidateAllowsSocketTimeoutSpecialValues(t *testing.T) {
	cfg := &Config{
		Addrs: []string{"127.0.0.1:6379"},
		Timeout: &TimeoutConfig{
			ReadTimeout:  time.Duration(-1),
			WriteTimeout: time.Duration(-2),
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsInvalidSocketTimeout(t *testing.T) {
	cfg := &Config{
		Addrs: []string{"127.0.0.1:6379"},
		Timeout: &TimeoutConfig{
			ReadTimeout:  time.Duration(-3),
			WriteTimeout: time.Duration(-3),
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}

	wantContains := []string{
		"redis timeout.read_timeout must be -2, -1, or greater than or equal to 0",
		"redis timeout.write_timeout must be -2, -1, or greater than or equal to 0",
	}
	for _, want := range wantContains {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Validate() error = %v, want to contain %q", err, want)
		}
	}
}

func TestConfigValidateRejectsInvalidAddr(t *testing.T) {
	cfg := &Config{
		Addrs: []string{"127.0.0.1"},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "addr must be host:port") {
		t.Fatalf("Validate() error = %v", err)
	}
}
