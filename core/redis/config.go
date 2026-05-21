package redis

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Mode 表示 Redis 部署拓扑模式。
type Mode string

const (
	// ModeStandalone 表示单节点 Redis，适合本地开发、小规模服务或由外部代理托管高可用的场景。
	ModeStandalone Mode = "standalone"

	// ModeSentinel 表示 Redis Sentinel 模式，通过哨兵发现主节点并处理故障转移。
	ModeSentinel Mode = "sentinel"

	// ModeCluster 表示 Redis Cluster 模式，客户端根据槽位路由请求。
	ModeCluster Mode = "cluster"
)

// Config 表示 Redis 客户端配置。
//
// 该结构体面向“应用配置文件”设计，只保留字符串、数字、布尔值、Duration 等
// 可序列化字段，不包含 Dialer、OnConnect、CredentialsProvider 等运行时代码钩子。
//
// 常规项目通常只需要配置 Mode、Addrs、DB、Username、Password、Timeout、Retry、
// Pool 和 TLS。ClientName、Protocol、Buffer、Monitoring、Sentinel、Cluster 等字段属于按需配置：
// 需要排查连接来源、兼容 RESP2、调优大吞吐读写、启用监控、启用哨兵或集群时再使用。
//
// 配置组字段使用指针是为了区分“整组未配置”和“整组参与覆盖”。例如 Timeout 为 nil
// 表示完全使用 go-redis 默认超时配置；Timeout 非 nil 时，组内字段会按 go-redis
// 自身的字段语义传递，字段零值通常仍表示使用 go-redis 默认值。
type Config struct {
	// Mode 指定 Redis 部署拓扑。常用字段。
	// 为空时建议由加载配置的一方默认为 ModeStandalone。
	Mode Mode `json:"mode" yaml:"mode" mapstructure:"mode"`

	// Addrs 是 Redis 节点地址列表，格式通常为 host:port。
	// standalone 模式只使用第一个地址；sentinel 模式表示哨兵地址；
	// cluster 模式表示集群种子节点地址。
	// 常用字段，生产环境应显式配置。
	Addrs []string `json:"addrs" yaml:"addrs" mapstructure:"addrs"`

	// Network 是连接网络类型，通常为 tcp；使用 Unix Socket 时为 unix。
	// 按需字段，大多数 TCP 部署可以留空并使用 go-redis 默认值。
	Network string `json:"network" yaml:"network" mapstructure:"network"`

	// DB 是 Redis 逻辑库编号，仅 standalone 和 sentinel 主从模式有效。
	// 常用字段；cluster 模式不支持多 DB，通常保持 0。
	DB int `json:"db" yaml:"db" mapstructure:"db"`

	// Username 和 Password 用于 Redis 服务端认证。
	// 常用字段。Redis ACL 场景配置 Username；传统 requirepass 场景通常只配置 Password。
	Username string `json:"username" yaml:"username" mapstructure:"username"`
	Password string `json:"password" yaml:"password" mapstructure:"password"`

	// ClientName 会在建连后通过 CLIENT SETNAME 写入，便于服务端排查连接来源。
	// 按需字段，建议生产服务配置为服务名或实例前缀。
	ClientName string `json:"client_name" yaml:"client_name" mapstructure:"client_name"`

	// Protocol 指定 RESP 协议版本，go-redis/v9 默认使用 RESP3。
	// 兼容性字段。需要兼容旧代理、旧 Redis 服务或依赖 RESP2 返回格式时设置为 2。
	Protocol int `json:"protocol" yaml:"protocol" mapstructure:"protocol"`

	// DisableIdentity 禁用 go-redis 建连时发送的客户端库身份信息。
	// 特殊字段。只有在 Redis 版本、代理或安全策略不接受客户端身份上报时才需要开启。
	DisableIdentity bool `json:"disable_identity" yaml:"disable_identity" mapstructure:"disable_identity"`

	// IdentitySuffix 为 go-redis 客户端身份信息追加后缀，用于区分服务或实例。
	// 按需字段，适合多服务共用同一 Redis 集群时辅助观测。
	IdentitySuffix string `json:"identity_suffix" yaml:"identity_suffix" mapstructure:"identity_suffix"`

	// UnstableResp3 启用不稳定 RESP3 结果模式，主要用于 RediSearch 等模块场景。
	// 特殊字段。普通 key-value、缓存、分布式锁等场景不需要开启。
	UnstableResp3 bool `json:"unstable_resp3" yaml:"unstable_resp3" mapstructure:"unstable_resp3"`

	// Timeout 是连接、读写和等待连接池的超时配置。常用配置组。
	// nil 表示使用 go-redis 默认超时配置。
	Timeout *TimeoutConfig `json:"timeout" yaml:"timeout" mapstructure:"timeout"`

	// Retry 是命令级重试配置。常用配置组，尤其适合网络偶发抖动场景。
	// nil 表示使用 go-redis 默认重试配置。
	Retry *RetryConfig `json:"retry" yaml:"retry" mapstructure:"retry"`

	// Pool 是连接池配置。常用配置组，高并发服务通常需要显式调优 Size 和空闲连接数。
	// nil 表示使用 go-redis 默认连接池配置。
	Pool *PoolConfig `json:"pool" yaml:"pool" mapstructure:"pool"`

	// Buffer 是单连接读写缓冲区配置。性能调优字段，大多数项目可使用 go-redis 默认值。
	// nil 表示使用 go-redis 默认缓冲区大小。
	Buffer *BufferConfig `json:"buffer" yaml:"buffer" mapstructure:"buffer"`

	// TLS 是 Redis TLS 配置。云 Redis、跨网络访问或合规场景常用；内网自建 Redis 可留空。
	// nil 或 Enabled 为 false 表示不启用 TLS。
	TLS *TLSConfig `json:"tls" yaml:"tls" mapstructure:"tls"`

	// Monitoring 是 Redis OpenTelemetry 监控配置。
	// nil 或内部开关全为 false 表示不启用 redisotel instrumentation。
	Monitoring *MonitoringConfig `json:"monitoring" yaml:"monitoring" mapstructure:"monitoring"`

	// Sentinel 仅在 ModeSentinel 下使用。
	// nil 表示不启用 Sentinel 专属配置。
	Sentinel *SentinelConfig `json:"sentinel" yaml:"sentinel" mapstructure:"sentinel"`

	// Cluster 仅在 ModeCluster 下使用。
	// nil 表示不启用 Cluster 专属配置。
	Cluster *ClusterConfig `json:"cluster" yaml:"cluster" mapstructure:"cluster"`
}

// Validate 校验 Redis 配置是否满足创建 go-redis/v9 客户端的基本要求。
//
// 该方法只做确定性的配置校验，不做网络探测，也不会检查 Redis 服务是否真实可达。
// Mode 为空时按 ModeStandalone 处理，与 Config.Mode 的默认值约定保持一致。
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("redis config is nil")
	}

	var errs []error
	mode := c.modeOrDefault()
	network := c.networkOrDefault()

	if !isValidMode(mode) {
		errs = append(errs, fmt.Errorf("redis mode must be one of %q, %q, %q", ModeStandalone, ModeSentinel, ModeCluster))
	}
	if !isValidNetwork(network) {
		errs = append(errs, fmt.Errorf("redis network must be one of tcp, tcp4, tcp6, unix"))
	}
	if len(c.Addrs) == 0 {
		errs = append(errs, errors.New("redis addrs must not be empty"))
	}
	for i, addr := range c.Addrs {
		if err := validateAddr(network, addr); err != nil {
			errs = append(errs, fmt.Errorf("redis addrs[%d]: %w", i, err))
		}
	}
	if mode == ModeStandalone && len(c.Addrs) > 1 {
		errs = append(errs, errors.New("redis standalone mode requires exactly one addr"))
	}
	if network == "unix" && mode != ModeStandalone {
		errs = append(errs, errors.New("redis unix network is only supported in standalone mode"))
	}
	if c.DB < 0 {
		errs = append(errs, errors.New("redis db must be greater than or equal to 0"))
	}
	if mode == ModeCluster && c.DB != 0 {
		errs = append(errs, errors.New("redis cluster mode requires db to be 0"))
	}
	if c.Protocol != 0 && c.Protocol != 2 && c.Protocol != 3 {
		errs = append(errs, errors.New("redis protocol must be 0, 2, or 3"))
	}

	if c.Timeout != nil {
		errs = append(errs, c.Timeout.validate()...)
	}
	if c.Retry != nil {
		errs = append(errs, c.Retry.validate()...)
	}
	if c.Pool != nil {
		errs = append(errs, c.Pool.validate()...)
	}
	if c.Buffer != nil {
		errs = append(errs, c.Buffer.validate()...)
	}
	if c.TLS != nil {
		errs = append(errs, c.TLS.validate()...)
	}
	if c.Monitoring != nil {
		errs = append(errs, c.Monitoring.validate()...)
	}

	switch mode {
	case ModeSentinel:
		if c.Sentinel == nil {
			errs = append(errs, errors.New("redis sentinel mode requires sentinel config"))
		} else {
			errs = append(errs, c.Sentinel.validate()...)
		}
	case ModeCluster:
		if c.Cluster != nil {
			errs = append(errs, c.Cluster.validate()...)
		}
		if c.Sentinel != nil {
			errs = append(errs, errors.New("redis cluster mode must not set sentinel config"))
		}
	case ModeStandalone:
		if c.Sentinel != nil {
			errs = append(errs, errors.New("redis standalone mode must not set sentinel config"))
		}
		if c.Cluster != nil {
			errs = append(errs, errors.New("redis standalone mode must not set cluster config"))
		}
	}

	return errors.Join(errs...)
}

func (c *Config) modeOrDefault() Mode {
	if c.Mode == "" {
		return ModeStandalone
	}
	return c.Mode
}

// networkOrDefault 返回 go-redis 默认使用的网络类型。
// 大多数 Redis 部署使用 TCP；只有本机 Unix Socket 场景才需要显式配置 unix。
func (c *Config) networkOrDefault() string {
	if c.Network == "" {
		return "tcp"
	}
	return c.Network
}

// isValidMode 校验当前包支持的 Redis 拓扑类型。
func isValidMode(mode Mode) bool {
	switch mode {
	case ModeStandalone, ModeSentinel, ModeCluster:
		return true
	default:
		return false
	}
}

// isValidNetwork 校验 net.Dial 支持且本包允许暴露到配置文件的网络类型。
func isValidNetwork(network string) bool {
	switch network {
	case "tcp", "tcp4", "tcp6", "unix":
		return true
	default:
		return false
	}
}

// validateAddr 校验 Redis 地址格式。
// TCP 地址要求 host:port；Unix Socket 地址只要求非空路径。
func validateAddr(network, addr string) error {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return errors.New("addr must not be empty")
	}
	if network == "unix" {
		return nil
	}

	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("addr must be host:port: %w", err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("port must be a number: %w", err)
	}
	if portNumber <= 0 || portNumber > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	return nil
}

// TimeoutConfig 表示 Redis 连接和命令超时配置。
//
// 这组字段属于生产服务常用配置。建议所有对 Redis 有明确 SLA 的服务显式配置
// DialTimeout、ReadTimeout、WriteTimeout 和 PoolTimeout，避免依赖库默认值导致行为不透明。
type TimeoutConfig struct {
	// DialTimeout 是建立 TCP/TLS 连接的超时时间。
	// 常用字段。网络故障时它决定建连失败返回的速度。
	DialTimeout time.Duration `json:"dial_timeout" yaml:"dial_timeout" mapstructure:"dial_timeout"`

	// DialerRetries 是建连失败后的重试次数。
	// 按需字段。服务启动或 Redis 短暂抖动时有用；强实时请求链路不宜设置过高。
	DialerRetries int `json:"dialer_retries" yaml:"dialer_retries" mapstructure:"dialer_retries"`

	// DialerRetryTimeout 是建连重试的固定退避时间。
	// 按需字段，通常与 DialerRetries 一起配置。
	DialerRetryTimeout time.Duration `json:"dialer_retry_timeout" yaml:"dialer_retry_timeout" mapstructure:"dialer_retry_timeout"`

	// ReadTimeout 是读取 Redis 响应的超时时间。
	// 常用字段。它会影响慢命令、大批量返回和阻塞命令的行为。
	// 支持 go-redis 特殊值：-1 表示不设置超时，-2 表示禁用 SetReadDeadline。
	ReadTimeout time.Duration `json:"read_timeout" yaml:"read_timeout" mapstructure:"read_timeout"`

	// WriteTimeout 是写入 Redis 请求的超时时间。
	// 常用字段。网络拥塞或连接异常时它决定写入失败返回的速度。
	// 支持 go-redis 特殊值：-1 表示不设置超时，-2 表示禁用 SetWriteDeadline。
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout" mapstructure:"write_timeout"`

	// PoolTimeout 是连接池耗尽时等待可用连接的超时时间。
	// 常用字段。高并发下它能帮助尽早暴露连接池不足或 Redis 变慢的问题。
	PoolTimeout time.Duration `json:"pool_timeout" yaml:"pool_timeout" mapstructure:"pool_timeout"`

	// ContextTimeoutEnabled 表示是否让 go-redis 同时尊重 context 的超时和截止时间。
	// 按需字段。HTTP/RPC 请求链路通常建议开启，后台批处理可按业务需求决定。
	ContextTimeoutEnabled bool `json:"context_timeout_enabled" yaml:"context_timeout_enabled" mapstructure:"context_timeout_enabled"`
}

// validate 校验超时配置。0 表示交给 go-redis 默认值；ReadTimeout 和 WriteTimeout
// 允许 go-redis 定义的 -1、-2 特殊值，其他负数视为非法配置。
func (c *TimeoutConfig) validate() []error {
	var errs []error
	if c.DialTimeout < 0 {
		errs = append(errs, errors.New("redis timeout.dial_timeout must be greater than or equal to 0"))
	}
	if c.DialerRetries < 0 {
		errs = append(errs, errors.New("redis timeout.dialer_retries must be greater than or equal to 0"))
	}
	if c.DialerRetryTimeout < 0 {
		errs = append(errs, errors.New("redis timeout.dialer_retry_timeout must be greater than or equal to 0"))
	}
	if !isValidSocketTimeout(c.ReadTimeout) {
		errs = append(errs, errors.New("redis timeout.read_timeout must be -2, -1, or greater than or equal to 0"))
	}
	if !isValidSocketTimeout(c.WriteTimeout) {
		errs = append(errs, errors.New("redis timeout.write_timeout must be -2, -1, or greater than or equal to 0"))
	}
	if c.PoolTimeout < 0 {
		errs = append(errs, errors.New("redis timeout.pool_timeout must be greater than or equal to 0"))
	}
	return errs
}

func isValidSocketTimeout(timeout time.Duration) bool {
	return timeout >= 0 || timeout == -1 || timeout == -2
}

// RetryConfig 表示命令级重试配置。
//
// 这组字段属于生产服务常用配置，但不应无脑加大重试次数。对非幂等 Lua 脚本、
// 队列消费、计数器等操作，需要结合业务语义评估重试风险。
type RetryConfig struct {
	// MaxRetries 是命令失败后的最大重试次数。go-redis 中 -1 表示禁用重试。
	// 常用字段。读请求和幂等写请求可以适当开启；强一致写链路应谨慎。
	MaxRetries int `json:"max_retries" yaml:"max_retries" mapstructure:"max_retries"`

	// MinRetryBackoff 是命令重试的最小退避时间。go-redis 中 -1 表示禁用退避。
	// 常用字段。用于避免瞬时故障时立即密集重试。
	MinRetryBackoff time.Duration `json:"min_retry_backoff" yaml:"min_retry_backoff" mapstructure:"min_retry_backoff"`

	// MaxRetryBackoff 是命令重试的最大退避时间。go-redis 中 -1 表示禁用退避。
	// 常用字段。用于限制退避上限，避免请求等待过久。
	MaxRetryBackoff time.Duration `json:"max_retry_backoff" yaml:"max_retry_backoff" mapstructure:"max_retry_backoff"`
}

// validate 校验重试配置。
// go-redis 使用 -1 表示禁用重试或禁用退避，因此这里允许 -1，但拒绝更小的值。
func (c *RetryConfig) validate() []error {
	var errs []error
	if c.MaxRetries < -1 {
		errs = append(errs, errors.New("redis retry.max_retries must be greater than or equal to -1"))
	}
	if c.MinRetryBackoff < -1 {
		errs = append(errs, errors.New("redis retry.min_retry_backoff must be greater than or equal to -1"))
	}
	if c.MaxRetryBackoff < -1 {
		errs = append(errs, errors.New("redis retry.max_retry_backoff must be greater than or equal to -1"))
	}
	if c.MinRetryBackoff >= 0 && c.MaxRetryBackoff >= 0 && c.MinRetryBackoff > c.MaxRetryBackoff {
		errs = append(errs, errors.New("redis retry.min_retry_backoff must be less than or equal to max_retry_backoff"))
	}
	return errs
}

// PoolConfig 表示 Redis 连接池配置。
//
// 这组字段属于生产服务常用配置。多数项目重点关注 Size、MinIdleConns、
// MaxIdleConns、MaxActiveConns 和 ConnMaxIdleTime；其他字段属于进一步调优。
type PoolConfig struct {
	// FIFO 指定连接池是否使用 FIFO 策略；false 表示使用 go-redis 默认的 LIFO。
	// 调优字段。通常保持默认即可，只有在连接复用公平性有明确诉求时再开启。
	FIFO bool `json:"fifo" yaml:"fifo" mapstructure:"fifo"`

	// Size 是基础连接池大小。cluster 模式下该值按节点生效。
	// 常用字段。高并发服务通常需要结合 QPS、命令耗时和实例数设置。
	Size int `json:"size" yaml:"size" mapstructure:"size"`

	// MaxConcurrentDials 限制并发建连 goroutine 数量。
	// 调优字段。用于控制 Redis 故障恢复或冷启动时的并发建连压力。
	MaxConcurrentDials int `json:"max_concurrent_dials" yaml:"max_concurrent_dials" mapstructure:"max_concurrent_dials"`

	// MinIdleConns 是最小空闲连接数。
	// 常用字段。可减少突发流量到来时的建连延迟。
	MinIdleConns int `json:"min_idle_conns" yaml:"min_idle_conns" mapstructure:"min_idle_conns"`

	// MaxIdleConns 是最大空闲连接数。
	// 常用字段。用于限制低峰期空闲连接占用。
	MaxIdleConns int `json:"max_idle_conns" yaml:"max_idle_conns" mapstructure:"max_idle_conns"`

	// MaxActiveConns 是连接池允许分配的最大连接数，0 表示不限制。
	// 常用字段。建议生产环境设置上限，避免 Redis 慢时应用无限扩张连接。
	MaxActiveConns int `json:"max_active_conns" yaml:"max_active_conns" mapstructure:"max_active_conns"`

	// ConnMaxIdleTime 是连接最大空闲时间。
	// 常用字段。用于回收长时间闲置连接。
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time" yaml:"conn_max_idle_time" mapstructure:"conn_max_idle_time"`

	// ConnMaxLifetime 是连接最大复用时长。
	// 按需字段。适合需要周期性刷新连接、负载均衡后端或规避长连接老化的场景。
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime" yaml:"conn_max_lifetime" mapstructure:"conn_max_lifetime"`

	// ConnMaxLifetimeJitter 为连接最大复用时长增加抖动，避免连接同时过期。
	// 调优字段。通常只在配置 ConnMaxLifetime 且实例连接数较多时使用。
	ConnMaxLifetimeJitter time.Duration `json:"conn_max_lifetime_jitter" yaml:"conn_max_lifetime_jitter" mapstructure:"conn_max_lifetime_jitter"`
}

// validate 校验连接池配置。连接数量和生命周期不能为负数，关联字段要保持基本一致。
func (c *PoolConfig) validate() []error {
	var errs []error
	if c.Size < 0 {
		errs = append(errs, errors.New("redis pool.size must be greater than or equal to 0"))
	}
	if c.MaxConcurrentDials < 0 {
		errs = append(errs, errors.New("redis pool.max_concurrent_dials must be greater than or equal to 0"))
	}
	if c.MinIdleConns < 0 {
		errs = append(errs, errors.New("redis pool.min_idle_conns must be greater than or equal to 0"))
	}
	if c.MaxIdleConns < 0 {
		errs = append(errs, errors.New("redis pool.max_idle_conns must be greater than or equal to 0"))
	}
	if c.MaxActiveConns < 0 {
		errs = append(errs, errors.New("redis pool.max_active_conns must be greater than or equal to 0"))
	}
	if c.MaxIdleConns > 0 && c.MinIdleConns > c.MaxIdleConns {
		errs = append(errs, errors.New("redis pool.min_idle_conns must be less than or equal to max_idle_conns"))
	}
	if c.MaxActiveConns > 0 && c.MinIdleConns > c.MaxActiveConns {
		errs = append(errs, errors.New("redis pool.min_idle_conns must be less than or equal to max_active_conns"))
	}
	if c.ConnMaxIdleTime < 0 {
		errs = append(errs, errors.New("redis pool.conn_max_idle_time must be greater than or equal to 0"))
	}
	if c.ConnMaxLifetime < 0 {
		errs = append(errs, errors.New("redis pool.conn_max_lifetime must be greater than or equal to 0"))
	}
	if c.ConnMaxLifetimeJitter < 0 {
		errs = append(errs, errors.New("redis pool.conn_max_lifetime_jitter must be greater than or equal to 0"))
	}
	if c.ConnMaxLifetime == 0 && c.ConnMaxLifetimeJitter > 0 {
		errs = append(errs, errors.New("redis pool.conn_max_lifetime_jitter requires conn_max_lifetime"))
	}
	return errs
}

// BufferConfig 表示每条连接上的读写缓冲区大小。
//
// 这组字段属于性能调优配置，不是日常必配项。只有在大 pipeline、大 value、
// 高吞吐批处理等场景观察到 buffer 相关瓶颈时才建议调整。
type BufferConfig struct {
	// ReadBufferSize 是每条连接的读缓冲区大小，单位为字节。
	ReadBufferSize int `json:"read_buffer_size" yaml:"read_buffer_size" mapstructure:"read_buffer_size"`

	// WriteBufferSize 是每条连接的写缓冲区大小，单位为字节。
	WriteBufferSize int `json:"write_buffer_size" yaml:"write_buffer_size" mapstructure:"write_buffer_size"`
}

// validate 校验缓冲区大小。0 表示使用 go-redis 默认值，负数没有明确语义。
func (c *BufferConfig) validate() []error {
	var errs []error
	if c.ReadBufferSize < 0 {
		errs = append(errs, errors.New("redis buffer.read_buffer_size must be greater than or equal to 0"))
	}
	if c.WriteBufferSize < 0 {
		errs = append(errs, errors.New("redis buffer.write_buffer_size must be greater than or equal to 0"))
	}
	return errs
}

// TLSConfig 表示 Redis TLS 配置。
//
// 这组字段在云 Redis、公网/跨 VPC 访问、双向 TLS 或合规要求下常用；
// 纯内网自建 Redis 通常可以关闭。
type TLSConfig struct {
	// Enabled 表示是否启用 TLS。
	// 常用字段。开启后客户端应构造 tls.Config 并传给 go-redis。
	Enabled bool `json:"enabled" yaml:"enabled" mapstructure:"enabled"`

	// ServerName 用于 TLS 证书校验和 SNI。
	// 常用字段。使用域名证书或云 Redis TLS 时建议显式配置。
	ServerName string `json:"server_name" yaml:"server_name" mapstructure:"server_name"`

	// InsecureSkipVerify 跳过服务端证书校验，通常只应在测试环境使用。
	// 特殊字段。生产环境不建议开启。
	InsecureSkipVerify bool `json:"insecure_skip_verify" yaml:"insecure_skip_verify" mapstructure:"insecure_skip_verify"`

	// CAFile 是自定义 CA 证书路径。
	// 按需字段。服务端证书由私有 CA 签发时使用。
	CAFile string `json:"ca_file" yaml:"ca_file" mapstructure:"ca_file"`

	// CertFile 和 KeyFile 是客户端证书路径，用于双向 TLS。
	// 按需字段。只有 Redis 服务端要求客户端证书认证时才需要配置。
	CertFile string `json:"cert_file" yaml:"cert_file" mapstructure:"cert_file"`
	KeyFile  string `json:"key_file" yaml:"key_file" mapstructure:"key_file"`
}

func (c *TLSConfig) validate() []error {
	var errs []error
	if !c.Enabled {
		return errs
	}
	if (c.CertFile == "") != (c.KeyFile == "") {
		errs = append(errs, errors.New("redis tls.cert_file and tls.key_file must be set together"))
	}
	return errs
}

// MonitoringConfig 表示 Redis OpenTelemetry 监控配置。
//
// go-redis/v9 的监控能力由 github.com/redis/go-redis/extra/redisotel/v9 提供。
// Prometheus 场景对应 MetricsEnabled：redisotel 先采集 OpenTelemetry metrics，
// 再由 OpenTelemetry Prometheus exporter 暴露给 Prometheus 抓取。
type MonitoringConfig struct {
	// TracingEnabled 表示是否启用 Redis 命令链路追踪。
	// 开启后 NewClient 应调用 redisotel.InstrumentTracing(client)。
	TracingEnabled bool `json:"tracing_enabled" yaml:"tracing_enabled" mapstructure:"tracing_enabled"`

	// MetricsEnabled 表示是否启用 Redis 命令和连接池指标采集。
	// Prometheus 监控 Redis 客户端时应开启该字段，并在应用层配置 Prometheus exporter。
	MetricsEnabled bool `json:"metrics_enabled" yaml:"metrics_enabled" mapstructure:"metrics_enabled"`
}

func (c *MonitoringConfig) validate() []error {
	return nil
}

// SentinelConfig 表示 Redis Sentinel 模式配置。
//
// 这组字段只在 ModeSentinel 下使用。普通单节点或 Redis Cluster 不需要配置。
type SentinelConfig struct {
	// MasterName 是 Sentinel 监控的主节点名称。
	// Sentinel 模式必填字段。
	MasterName string `json:"master_name" yaml:"master_name" mapstructure:"master_name"`

	// Username 和 Password 用于连接 Sentinel 自身的认证。
	// 按需字段。注意它们不是 Redis 数据节点的认证，数据节点认证使用 Config.Username/Password。
	Username string `json:"username" yaml:"username" mapstructure:"username"`
	Password string `json:"password" yaml:"password" mapstructure:"password"`

	// ReplicaOnly 表示优先把命令发送到副本节点。
	// 特殊字段。通常用于只读客户端，不适合包含写命令的业务客户端；当没有可用副本时，
	// go-redis 会回退到主节点。
	ReplicaOnly bool `json:"replica_only" yaml:"replica_only" mapstructure:"replica_only"`

	// UseDisconnectedReplicas 允许在没有可连接副本时使用与主节点断开的副本。
	// 特殊字段。可能读取到更旧的数据，只有业务明确允许时再开启。
	UseDisconnectedReplicas bool `json:"use_disconnected_replicas" yaml:"use_disconnected_replicas" mapstructure:"use_disconnected_replicas"`
}

func (c *SentinelConfig) validate() []error {
	var errs []error
	if strings.TrimSpace(c.MasterName) == "" {
		errs = append(errs, errors.New("redis sentinel.master_name must not be empty"))
	}
	return errs
}

// ClusterConfig 表示 Redis Cluster 模式配置。
//
// 这组字段只在 ModeCluster 下使用。普通单节点和 Sentinel 主从模式不需要配置。
type ClusterConfig struct {
	// MaxRedirects 是 MOVED/ASK 等集群重定向的最大处理次数。
	// 常用字段。用于控制槽位迁移、扩缩容期间的重定向重试上限。
	// 0 表示使用 go-redis 默认值；-1 表示禁用重定向。
	MaxRedirects int `json:"max_redirects" yaml:"max_redirects" mapstructure:"max_redirects"`

	// ReadOnly 允许只读命令路由到副本节点。
	// 读路由字段。只有明确允许读副本且能接受复制延迟时才建议开启。
	ReadOnly bool `json:"read_only" yaml:"read_only" mapstructure:"read_only"`

	// RouteByLatency 允许只读命令路由到延迟最低的主节点或副本。
	// 读路由字段。适合多副本、多可用区部署下优化只读延迟。
	RouteByLatency bool `json:"route_by_latency" yaml:"route_by_latency" mapstructure:"route_by_latency"`

	// RouteRandomly 允许只读命令随机路由到主节点或副本。
	// 读路由字段。适合分散只读流量，但需要接受副本延迟。
	RouteRandomly bool `json:"route_randomly" yaml:"route_randomly" mapstructure:"route_randomly"`

	// FailingTimeoutSeconds 是节点被标记失败后的避让秒数。
	// 调优字段。用于控制客户端在节点异常后多久重新尝试该节点。
	FailingTimeoutSeconds int `json:"failing_timeout_seconds" yaml:"failing_timeout_seconds" mapstructure:"failing_timeout_seconds"`

	// ClusterStateReloadInterval 是集群槽位状态刷新间隔。
	// 调优字段。集群拓扑频繁变化时可适当缩短，稳定集群通常使用默认值。
	ClusterStateReloadInterval time.Duration `json:"cluster_state_reload_interval" yaml:"cluster_state_reload_interval" mapstructure:"cluster_state_reload_interval"`
}

func (c *ClusterConfig) validate() []error {
	var errs []error
	if c.MaxRedirects < -1 {
		errs = append(errs, errors.New("redis cluster.max_redirects must be greater than or equal to -1"))
	}
	if c.FailingTimeoutSeconds < 0 {
		errs = append(errs, errors.New("redis cluster.failing_timeout_seconds must be greater than or equal to 0"))
	}
	if c.ClusterStateReloadInterval < 0 {
		errs = append(errs, errors.New("redis cluster.cluster_state_reload_interval must be greater than or equal to 0"))
	}
	return errs
}
