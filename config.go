package fox

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Mode 表示 HTTP server 运行模式。
type Mode string

const (
	// ModeDebug 表示调试模式，输出详细日志和错误堆栈，适合本地开发。
	ModeDebug Mode = "debug"
	// ModeRelease 表示生产模式，关闭调试信息，性能优先。
	ModeRelease Mode = "release"
	// ModeTest 表示测试模式，用于单元测试和集成测试。
	ModeTest Mode = "test"
)

const defaultAddr = ":8080"
const defaultShutdownTimeout = 30 * time.Second

// Config 表示 HTTP server 核心配置。
type Config struct {
	// Mode 为空时使用 ModeRelease。
	Mode Mode `json:"mode" yaml:"mode" mapstructure:"mode"`
	// Addr 监听地址，支持 ":8080"、"127.0.0.1:8080" 和 "8080"。
	Addr string `json:"addr" yaml:"addr" mapstructure:"addr"`
	// ReadTimeout 读取请求的超时时间，包含 Header 和 Body。
	ReadTimeout time.Duration `json:"read_timeout" yaml:"read_timeout" mapstructure:"read_timeout"`
	// WriteTimeout 写入响应的超时时间。
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout" mapstructure:"write_timeout"`
	// ShutdownTimeout 收到退出信号后的优雅关机等待时间，0 使用默认值。
	ShutdownTimeout time.Duration `json:"shutdown_timeout" yaml:"shutdown_timeout" mapstructure:"shutdown_timeout"`
	// MaxHeaderBytes 请求 Header 的最大字节数，0 表示使用标准库默认值。
	MaxHeaderBytes int `json:"max_header_bytes" yaml:"max_header_bytes" mapstructure:"max_header_bytes"`
	// MaxMultipartMemory multipart/form 表单上传时使用的内存上限，0 使用 Gin 默认值。
	MaxMultipartMemory int `json:"max_multipart_memory" yaml:"max_multipart_memory" mapstructure:"max_multipart_memory"`
	// TLS HTTPS 配置，为 nil 时不启用 TLS。
	TLS *TLSConfig `json:"tls,omitempty" yaml:"tls,omitempty" mapstructure:"tls"`
	// TrustedProxies 信任的反向代理 IP 或 CIDR 列表。
	TrustedProxies []string `json:"trusted_proxies,omitempty" yaml:"trusted_proxies,omitempty" mapstructure:"trusted_proxies"`
	// PrintRoutes 控制启动时是否打印路由表。
	// nil 时，debug/test 默认启用，release 默认关闭。
	PrintRoutes *bool `json:"print_routes,omitempty" yaml:"print_routes,omitempty" mapstructure:"print_routes"`
	// EnableLogger 兼容旧配置名，目前等同于 PrintRoutes。
	// Deprecated: use PrintRoutes.
	EnableLogger *bool `json:"enable_logger,omitempty" yaml:"enable_logger,omitempty" mapstructure:"enable_logger"`
	// UseH2C 是否允许明文 HTTP/2 (h2c) 升级。
	UseH2C bool `json:"use_h2c" yaml:"use_h2c" mapstructure:"use_h2c"`
}

// TLSConfig 表示 HTTPS / TLS 配置。
type TLSConfig struct {
	// CertFile 证书文件路径（PEM 格式）。
	CertFile string `json:"cert_file" yaml:"cert_file" mapstructure:"cert_file"`
	// KeyFile 私钥文件路径（PEM 格式）。
	KeyFile string `json:"key_file" yaml:"key_file" mapstructure:"key_file"`
	// MinVersion 允许的最低 TLS 版本，0 表示使用 Go 默认值。
	MinVersion uint16 `json:"min_version" yaml:"min_version" mapstructure:"min_version"`
	// CipherSuites 允许的密码套件列表，为空时使用 Go 默认安全套件。
	CipherSuites []uint16 `json:"cipher_suites,omitempty" yaml:"cipher_suites,omitempty" mapstructure:"cipher_suites"`
	// Config 底层 *tls.Config，设置后将覆盖 CertFile/KeyFile/MinVersion/CipherSuites。
	Config *tls.Config `json:"-" yaml:"-" mapstructure:"-"`
}

// Validate 校验 HTTP server 配置是否满足启动的基本要求。
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("server config is nil")
	}

	var errs []error
	if !isValidMode(c.modeOrDefault()) {
		errs = append(errs, fmt.Errorf("server mode must be one of %q, %q, %q", ModeDebug, ModeRelease, ModeTest))
	}
	if err := validateAddr(c.Addr); err != nil {
		errs = append(errs, fmt.Errorf("server addr: %w", err))
	}
	if c.ReadTimeout < 0 {
		errs = append(errs, errors.New("server read_timeout must be greater than or equal to 0"))
	}
	if c.WriteTimeout < 0 {
		errs = append(errs, errors.New("server write_timeout must be greater than or equal to 0"))
	}
	if c.ShutdownTimeout < 0 {
		errs = append(errs, errors.New("server shutdown_timeout must be greater than or equal to 0"))
	}
	if c.MaxHeaderBytes < 0 {
		errs = append(errs, errors.New("server max_header_bytes must be greater than or equal to 0"))
	}
	if c.MaxMultipartMemory < 0 {
		errs = append(errs, errors.New("server max_multipart_memory must be greater than or equal to 0"))
	}
	if c.UseH2C && c.TLS != nil {
		errs = append(errs, errors.New("server use_h2c and tls cannot be enabled at the same time"))
	}
	if c.TLS != nil {
		errs = append(errs, c.TLS.validate()...)
	}
	for _, proxy := range c.TrustedProxies {
		if strings.TrimSpace(proxy) == "" {
			errs = append(errs, errors.New("server trusted_proxies must not contain empty string"))
			break
		}
		if _, _, err := net.ParseCIDR(proxy); err != nil {
			if ip := net.ParseIP(proxy); ip == nil {
				errs = append(errs, fmt.Errorf("server trusted_proxies: %q is not a valid IP or CIDR", proxy))
			}
		}
	}
	return errors.Join(errs...)
}

func defaultConfig(cfg *Config) Config {
	if cfg == nil {
		cfg = &Config{}
	}
	cfgCopy := *cfg
	if strings.TrimSpace(cfgCopy.Addr) == "" {
		cfgCopy.Addr = defaultAddr
	}
	if cfgCopy.ShutdownTimeout == 0 {
		cfgCopy.ShutdownTimeout = defaultShutdownTimeout
	}
	return cfgCopy
}

func (c *Config) modeOrDefault() Mode {
	if c == nil || c.Mode == "" {
		return ModeRelease
	}
	return c.Mode
}

func (c *Config) printRoutesEnabled() bool {
	if c == nil {
		return false
	}
	if c.PrintRoutes != nil {
		return *c.PrintRoutes
	}
	if c.EnableLogger != nil {
		return *c.EnableLogger
	}
	return c.modeOrDefault() != ModeRelease
}

func (c *TLSConfig) validate() []error {
	var errs []error
	if c.Config == nil {
		if strings.TrimSpace(c.CertFile) == "" {
			errs = append(errs, errors.New("server tls.cert_file must not be empty"))
		}
		if strings.TrimSpace(c.KeyFile) == "" {
			errs = append(errs, errors.New("server tls.key_file must not be empty"))
		}
	}
	if c.MinVersion != 0 && !isValidTLSVersion(c.MinVersion) {
		errs = append(errs, fmt.Errorf("server tls.min_version must be one of 0x%04X, 0x%04X, 0x%04X, 0x%04X",
			tls.VersionTLS10, tls.VersionTLS11, tls.VersionTLS12, tls.VersionTLS13))
	}
	for _, cs := range c.CipherSuites {
		if !isValidCipherSuite(cs) {
			errs = append(errs, fmt.Errorf("server tls.cipher_suites contains unsupported value 0x%04X", cs))
		}
	}
	return errs
}

func isValidMode(m Mode) bool {
	switch m {
	case ModeDebug, ModeRelease, ModeTest:
		return true
	default:
		return false
	}
}

func isValidTLSVersion(v uint16) bool {
	switch v {
	case tls.VersionTLS10, tls.VersionTLS11, tls.VersionTLS12, tls.VersionTLS13:
		return true
	default:
		return false
	}
}

func isValidCipherSuite(cs uint16) bool {
	for _, s := range tls.CipherSuites() {
		if s.ID == cs {
			return true
		}
	}
	for _, s := range tls.InsecureCipherSuites() {
		if s.ID == cs {
			return true
		}
	}
	return false
}

func validateAddr(addr string) error {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return errors.New("must not be empty")
	}
	if port, err := strconv.Atoi(addr); err == nil {
		if port < 0 || port > 65535 {
			return fmt.Errorf("port must be between 0 and 65535, got %d", port)
		}
		return nil
	}
	_, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("must be host:port format, got %q", addr)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return fmt.Errorf("port must be a number, got %q", portText)
	}
	if port < 0 || port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535, got %d", port)
	}
	return nil
}

func normalizeAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if _, err := strconv.Atoi(addr); err == nil {
		return ":" + addr
	}
	return addr
}
