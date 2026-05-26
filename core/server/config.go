package server

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/surge-go/fox/pkg/openapi"
)

// Mode 表示 Gin 运行模式。
type Mode string

const (
	// ModeDebug 表示调试模式，输出详细日志和错误堆栈，适合本地开发。
	ModeDebug Mode = "debug"

	// ModeRelease 表示生产模式，关闭调试信息，性能最优。
	ModeRelease Mode = "release"

	// ModeTest 表示测试模式，用于单元测试和集成测试。
	ModeTest Mode = "test"
)

// Config 表示基于 Gin 的 HTTP 服务器配置。
type Config struct {
	// Mode 为空时建议使用 ModeRelease；本地开发可配置为 ModeDebug。
	Mode Mode `json:"mode" yaml:"mode" mapstructure:"mode"`
	// Addr 监听地址，格式为 "host:port"，如 ":8080"。
	Addr string `json:"addr" yaml:"addr" mapstructure:"addr"`
	// ReadTimeout 读取请求的超时时间，包含读取 Header 和 Body 的完整耗时。
	ReadTimeout time.Duration `json:"read_timeout" yaml:"read_timeout" mapstructure:"read_timeout"`
	// WriteTimeout 写入响应的超时时间，从请求头读取完毕后开始计时。
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout" mapstructure:"write_timeout"`
	// MaxHeaderBytes 请求 Header 的最大字节数，0 表示使用 http.DefaultMaxHeaderBytes (1MB)。
	MaxHeaderBytes int `json:"max_header_bytes" yaml:"max_header_bytes" mapstructure:"max_header_bytes"`
	// MaxMultipartMemory multipart/form 表单上传时使用的内存上限（字节），超出部分写入临时文件。
	MaxMultipartMemory int `json:"max_multipart_memory" yaml:"max_multipart_memory" mapstructure:"max_multipart_memory"`
	// TLS HTTPS 配置，为 nil 时表示不启用 TLS。
	TLS *TLSConfig `json:"tls,omitempty" yaml:"tls,omitempty" mapstructure:"tls"`
	// TrustedProxies 信任的反向代理 IP 或 CIDR 列表（如 "10.0.0.0/8", "127.0.0.1"）。
	// 为空时不信任 X-Forwarded-For / X-Real-IP 等代理头；生产环境建议显式配置。
	TrustedProxies []string `json:"trusted_proxies,omitempty" yaml:"trusted_proxies,omitempty" mapstructure:"trusted_proxies"`
	// EnableLogger 控制是否注册内置日志中间件。
	// 不配置时，非 release 模式默认启用，release 模式默认关闭。
	EnableLogger *bool `json:"enable_logger,omitempty" yaml:"enable_logger,omitempty" mapstructure:"enable_logger"`
	// UseH2C 是否允许明文 HTTP/2 (h2c) 升级。
	// 开启后服务端在不启用 TLS 的情况下也可接受 HTTP/2 连接，适用于内部服务间通信。
	UseH2C bool `json:"use_h2c" yaml:"use_h2c" mapstructure:"use_h2c"`
	// OpenAPI 为空时不启用自动文档收集。
	OpenAPI *OpenAPIConfig `json:"openapi,omitempty" yaml:"openapi,omitempty" mapstructure:"openapi"`
}

// OpenAPIConfig 表示 server 自动生成 OpenAPI 文档的配置。
type OpenAPIConfig struct {
	Info         openapi.Info
	Servers      []openapi.Server
	Tags         []openapi.Tag
	TagResolvers []openapi.TagResolver
	// ResponseDescriptions 配置默认响应描述，支持国际化。
	// 如果为空，使用英文默认值。
	ResponseDescriptions ResponseDescriptions
}

// ResponseDescriptions 定义 OpenAPI 文档中默认响应的描述文本。
type ResponseDescriptions struct {
	Success             string // 200 响应描述，默认 "Success"
	BadRequest          string // 400 响应描述，默认 "Bad Request"
	InternalServerError string // 500 响应描述，默认 "Internal Server Error"
}

// Validate 校验 HTTP 服务器配置是否满足启动的基本要求。
//
// 该方法只做确定性的静态配置校验，不会打开网络端口，也不会检查证书文件是否真实存在。
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

func (c *Config) modeOrDefault() Mode {
	if c == nil || c.Mode == "" {
		return ModeRelease
	}
	return c.Mode
}

func (c *Config) loggerEnabled() bool {
	if c == nil {
		return false
	}
	if c.EnableLogger != nil {
		return *c.EnableLogger
	}
	return c.modeOrDefault() != ModeRelease
}

// TLSConfig HTTPS / TLS 相关配置。
type TLSConfig struct {
	// CertFile 证书文件路径（PEM 格式）。
	CertFile string `json:"cert_file" yaml:"cert_file" mapstructure:"cert_file"`
	// KeyFile 私钥文件路径（PEM 格式）。
	KeyFile string `json:"key_file" yaml:"key_file" mapstructure:"key_file"`
	// MinVersion 允许的最低 TLS 版本，0 表示使用 Go 默认值（TLS 1.2）。
	MinVersion uint16 `json:"min_version" yaml:"min_version" mapstructure:"min_version"`
	// CipherSuites 允许的密码套件列表，为空时使用 Go 默认安全套件。
	CipherSuites []uint16 `json:"cipher_suites,omitempty" yaml:"cipher_suites,omitempty" mapstructure:"cipher_suites"`
	// Config 底层 *tls.Config，设置后将覆盖 CertFile/KeyFile/MinVersion/CipherSuites。
	Config *tls.Config `json:"-" yaml:"-" mapstructure:"-"`
}

// validate 校验 TLS 配置。
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
		errs = append(errs, fmt.Errorf("server tls.min_version must be one of 0x%04X (TLS1.0), 0x%04X (TLS1.1), 0x%04X (TLS1.2), 0x%04X (TLS1.3)",
			tls.VersionTLS10, tls.VersionTLS11, tls.VersionTLS12, tls.VersionTLS13))
	}

	for _, cs := range c.CipherSuites {
		if !isValidCipherSuite(cs) {
			errs = append(errs, fmt.Errorf("server tls.cipher_suites contains unsupported value 0x%04X", cs))
		}
	}

	return errs
}

// isValidMode 校验 Gin 运行模式。
func isValidMode(m Mode) bool {
	switch m {
	case ModeDebug, ModeRelease, ModeTest:
		return true
	default:
		return false
	}
}

// isValidTLSVersion 校验 TLS 版本号是否为 Go 标准库支持的值。
func isValidTLSVersion(v uint16) bool {
	switch v {
	case tls.VersionTLS10, tls.VersionTLS11, tls.VersionTLS12, tls.VersionTLS13:
		return true
	default:
		return false
	}
}

// isValidCipherSuite 校验密码套件是否为 crypto/tls 支持的安全套件。
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

// validateAddr 校验监听地址格式，支持 "host:port"、":port" 和 "port"。
func validateAddr(addr string) error {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return errors.New("must not be empty")
	}

	// 纯数字视为端口号。
	if port, err := strconv.Atoi(addr); err == nil {
		if port < 0 || port > 65535 {
			return fmt.Errorf("port must be between 0 and 65535, got %d", port)
		}
		return nil
	}

	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("must be host:port format, got %q", addr)
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
