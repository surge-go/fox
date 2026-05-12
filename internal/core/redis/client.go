package redis

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/redis/go-redis/extra/redisotel/v9"
	goredis "github.com/redis/go-redis/v9"
)

// Client 是 go-redis/v9 提供的通用客户端接口。
//
// 使用类型别名而不是自定义大接口，是为了直接复用 go-redis 的命令集合、
// pipeline、transaction、script、pub/sub 等能力，避免在本包重复维护一份
// Redis 命令 API。NewClient 返回的实际类型取决于 Config.Mode：
//   - ModeStandalone 返回 *redis.Client。
//   - ModeSentinel 返回 *redis.Client，内部通过 Sentinel 发现主从节点。
//   - ModeCluster 返回 *redis.ClusterClient。
//
// 业务层如果需要隔离缓存实现，建议按业务语义定义小接口，例如 UserCache、
// TokenStore，而不是在基础设施层重新抽象完整 Redis 客户端。
type Client = goredis.UniversalClient

// NewClient 根据 Config 创建 Redis 客户端。
//
// 函数会先执行 Config.Validate，随后根据部署拓扑创建对应的 go-redis 客户端，
// 最后按 MonitoringConfig 注册 redisotel tracing/metrics instrumentation。
//
// NewClient 不会主动执行 PING 或其他网络探测。这样做可以让“创建客户端”和
// “确认 Redis 当前可用”保持分离：前者是配置和对象初始化，后者通常属于应用
// 启动探测、readiness probe 或健康检查逻辑。调用方在需要强依赖 Redis 启动时，
// 可以在 NewClient 成功后自行执行 Ping(ctx)。
//
// 如果 redisotel 注册失败，函数会关闭已经创建的客户端并返回错误，避免调用方
// 拿到一个处于部分初始化状态的客户端。
func NewClient(cfg *Config) (Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	var (
		client Client
		err    error
	)

	switch cfg.modeOrDefault() {
	case ModeStandalone:
		client, err = newStandaloneClient(cfg)
	case ModeSentinel:
		client, err = newSentinelClient(cfg)
	case ModeCluster:
		client, err = newClusterClient(cfg)
	default:
		return nil, fmt.Errorf("unsupported redis mode %q", cfg.Mode)
	}
	if err != nil {
		return nil, err
	}

	if err := instrumentClient(client, cfg.Monitoring); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// newStandaloneClient 创建单节点 Redis 客户端。
//
// standalone 模式只使用 Config.Addrs 的第一个地址；如果需要代理、VIP、云厂商
// 单入口高可用，也应在配置上表现为 standalone，由外部入口负责故障转移。
//
// 该模式支持普通 TCP、tcp4/tcp6 和 Unix Socket。DB 配置在该模式下有效。
func newStandaloneClient(cfg *Config) (Client, error) {
	tlsConfig, err := cfg.buildTLSConfig()
	if err != nil {
		return nil, err
	}

	opt := &goredis.Options{
		Network:         cfg.networkOrDefault(),
		Addr:            cfg.Addrs[0],
		DB:              cfg.DB,
		Username:        cfg.Username,
		Password:        cfg.Password,
		ClientName:      cfg.ClientName,
		Protocol:        cfg.Protocol,
		DisableIdentity: cfg.DisableIdentity,
		IdentitySuffix:  cfg.IdentitySuffix,
		UnstableResp3:   cfg.UnstableResp3,
		TLSConfig:       tlsConfig,
	}
	applyTimeoutOptions(opt, cfg.Timeout)
	applyRetryOptions(opt, cfg.Retry)
	applyPoolOptions(opt, cfg.Pool)
	applyBufferOptions(opt, cfg.Buffer)

	return goredis.NewClient(opt), nil
}

// newSentinelClient 创建 Redis Sentinel 故障转移客户端。
//
// Config.Addrs 在该模式下表示 Sentinel 地址列表，Redis 数据节点的主库地址
// 由 Sentinel 根据 MasterName 动态发现。Sentinel 自身认证和数据节点认证分别
// 使用 SentinelConfig.Username/Password 与 Config.Username/Password。
//
// go-redis 的 FailoverClient 对外仍表现为 *redis.Client，但会在内部维护
// Sentinel 发现和主库切换。ReplicaOnly 开启后会优先连接副本节点，适合只读
// 客户端；普通读写客户端应保持关闭。业务侧仍应通过返回的 Client 接口使用它，
// 不需要依赖具体类型。
func newSentinelClient(cfg *Config) (Client, error) {
	tlsConfig, err := cfg.buildTLSConfig()
	if err != nil {
		return nil, err
	}

	opt := &goredis.FailoverOptions{
		MasterName:              cfg.Sentinel.MasterName,
		SentinelAddrs:           cfg.Addrs,
		SentinelUsername:        cfg.Sentinel.Username,
		SentinelPassword:        cfg.Sentinel.Password,
		DB:                      cfg.DB,
		Username:                cfg.Username,
		Password:                cfg.Password,
		ClientName:              cfg.ClientName,
		Protocol:                cfg.Protocol,
		DisableIdentity:         cfg.DisableIdentity,
		IdentitySuffix:          cfg.IdentitySuffix,
		UnstableResp3:           cfg.UnstableResp3,
		ReplicaOnly:             cfg.Sentinel.ReplicaOnly,
		UseDisconnectedReplicas: cfg.Sentinel.UseDisconnectedReplicas,
		TLSConfig:               tlsConfig,
	}
	applyFailoverTimeoutOptions(opt, cfg.Timeout)
	applyFailoverRetryOptions(opt, cfg.Retry)
	applyFailoverPoolOptions(opt, cfg.Pool)
	applyFailoverBufferOptions(opt, cfg.Buffer)

	return goredis.NewFailoverClient(opt), nil
}

// newClusterClient 创建 Redis Cluster 客户端。
//
// Config.Addrs 在该模式下表示集群种子节点地址。go-redis 会从种子节点加载槽位
// 拓扑，并在 MOVED/ASK 重定向或拓扑刷新后更新路由。
//
// Redis Cluster 不支持多逻辑库，Config.Validate 会要求 DB 为 0。PoolConfig 中
// 的连接池大小在 go-redis 中按集群节点分别生效，整体连接数需要结合节点数评估。
func newClusterClient(cfg *Config) (Client, error) {
	tlsConfig, err := cfg.buildTLSConfig()
	if err != nil {
		return nil, err
	}
	cluster := cfg.clusterConfig()

	opt := &goredis.ClusterOptions{
		Addrs:                      cfg.Addrs,
		Username:                   cfg.Username,
		Password:                   cfg.Password,
		ClientName:                 cfg.ClientName,
		Protocol:                   cfg.Protocol,
		DisableIdentity:            cfg.DisableIdentity,
		IdentitySuffix:             cfg.IdentitySuffix,
		UnstableResp3:              cfg.UnstableResp3,
		MaxRedirects:               cluster.MaxRedirects,
		ReadOnly:                   cluster.ReadOnly,
		RouteByLatency:             cluster.RouteByLatency,
		RouteRandomly:              cluster.RouteRandomly,
		FailingTimeoutSeconds:      cluster.FailingTimeoutSeconds,
		ClusterStateReloadInterval: cluster.ClusterStateReloadInterval,
		TLSConfig:                  tlsConfig,
	}
	applyClusterTimeoutOptions(opt, cfg.Timeout)
	applyClusterRetryOptions(opt, cfg.Retry)
	applyClusterPoolOptions(opt, cfg.Pool)
	applyClusterBufferOptions(opt, cfg.Buffer)

	return goredis.NewClusterClient(opt), nil
}

// clusterConfig 返回非 nil 的集群配置。
// Cluster 配置未显式提供时使用零值，让 go-redis 自己应用默认重定向和刷新策略。
func (c *Config) clusterConfig() *ClusterConfig {
	if c.Cluster != nil {
		return c.Cluster
	}
	return &ClusterConfig{}
}

// applyTimeoutOptions 将通用超时配置映射到 standalone 客户端选项。
// cfg 为 nil 时保持 go-redis 默认值；非 nil 时字段零值也会被显式写入。
func applyTimeoutOptions(opt *goredis.Options, cfg *TimeoutConfig) {
	if cfg == nil {
		return
	}
	opt.DialTimeout = cfg.DialTimeout
	opt.DialerRetries = cfg.DialerRetries
	opt.DialerRetryTimeout = cfg.DialerRetryTimeout
	opt.ReadTimeout = cfg.ReadTimeout
	opt.WriteTimeout = cfg.WriteTimeout
	opt.PoolTimeout = cfg.PoolTimeout
	opt.ContextTimeoutEnabled = cfg.ContextTimeoutEnabled
}

// applyRetryOptions 将命令重试配置映射到 standalone 客户端选项。
// go-redis 使用 -1 表示禁用重试或退避，Config.Validate 已保留该语义。
func applyRetryOptions(opt *goredis.Options, cfg *RetryConfig) {
	if cfg == nil {
		return
	}
	opt.MaxRetries = cfg.MaxRetries
	opt.MinRetryBackoff = cfg.MinRetryBackoff
	opt.MaxRetryBackoff = cfg.MaxRetryBackoff
}

// applyPoolOptions 将连接池配置映射到 standalone 客户端选项。
// 连接池参数直接影响并发能力和 Redis 连接数，生产配置应结合实例数统一评估。
func applyPoolOptions(opt *goredis.Options, cfg *PoolConfig) {
	if cfg == nil {
		return
	}
	opt.PoolFIFO = cfg.FIFO
	opt.PoolSize = cfg.Size
	opt.MaxConcurrentDials = cfg.MaxConcurrentDials
	opt.MinIdleConns = cfg.MinIdleConns
	opt.MaxIdleConns = cfg.MaxIdleConns
	opt.MaxActiveConns = cfg.MaxActiveConns
	opt.ConnMaxIdleTime = cfg.ConnMaxIdleTime
	opt.ConnMaxLifetime = cfg.ConnMaxLifetime
	opt.ConnMaxLifetimeJitter = cfg.ConnMaxLifetimeJitter
}

// applyBufferOptions 将读写缓冲区配置映射到 standalone 客户端选项。
// 只有大 pipeline、大 value 或高吞吐批处理场景通常才需要显式调优。
func applyBufferOptions(opt *goredis.Options, cfg *BufferConfig) {
	if cfg == nil {
		return
	}
	opt.ReadBufferSize = cfg.ReadBufferSize
	opt.WriteBufferSize = cfg.WriteBufferSize
}

// applyFailoverTimeoutOptions 将通用超时配置映射到 Sentinel failover 客户端选项。
// 这些超时会影响 Sentinel 发现连接和 Redis 数据节点连接。
func applyFailoverTimeoutOptions(opt *goredis.FailoverOptions, cfg *TimeoutConfig) {
	if cfg == nil {
		return
	}
	opt.DialTimeout = cfg.DialTimeout
	opt.DialerRetries = cfg.DialerRetries
	opt.DialerRetryTimeout = cfg.DialerRetryTimeout
	opt.ReadTimeout = cfg.ReadTimeout
	opt.WriteTimeout = cfg.WriteTimeout
	opt.PoolTimeout = cfg.PoolTimeout
	opt.ContextTimeoutEnabled = cfg.ContextTimeoutEnabled
}

// applyFailoverRetryOptions 将命令重试配置映射到 Sentinel failover 客户端选项。
func applyFailoverRetryOptions(opt *goredis.FailoverOptions, cfg *RetryConfig) {
	if cfg == nil {
		return
	}
	opt.MaxRetries = cfg.MaxRetries
	opt.MinRetryBackoff = cfg.MinRetryBackoff
	opt.MaxRetryBackoff = cfg.MaxRetryBackoff
}

// applyFailoverPoolOptions 将连接池配置映射到 Sentinel failover 客户端选项。
// 发生主从切换后，go-redis 会按这些参数为新的目标节点维护连接池。
func applyFailoverPoolOptions(opt *goredis.FailoverOptions, cfg *PoolConfig) {
	if cfg == nil {
		return
	}
	opt.PoolFIFO = cfg.FIFO
	opt.PoolSize = cfg.Size
	opt.MaxConcurrentDials = cfg.MaxConcurrentDials
	opt.MinIdleConns = cfg.MinIdleConns
	opt.MaxIdleConns = cfg.MaxIdleConns
	opt.MaxActiveConns = cfg.MaxActiveConns
	opt.ConnMaxIdleTime = cfg.ConnMaxIdleTime
	opt.ConnMaxLifetime = cfg.ConnMaxLifetime
	opt.ConnMaxLifetimeJitter = cfg.ConnMaxLifetimeJitter
}

// applyFailoverBufferOptions 将读写缓冲区配置映射到 Sentinel failover 客户端选项。
func applyFailoverBufferOptions(opt *goredis.FailoverOptions, cfg *BufferConfig) {
	if cfg == nil {
		return
	}
	opt.ReadBufferSize = cfg.ReadBufferSize
	opt.WriteBufferSize = cfg.WriteBufferSize
}

// applyClusterTimeoutOptions 将通用超时配置映射到 Redis Cluster 客户端选项。
func applyClusterTimeoutOptions(opt *goredis.ClusterOptions, cfg *TimeoutConfig) {
	if cfg == nil {
		return
	}
	opt.DialTimeout = cfg.DialTimeout
	opt.DialerRetries = cfg.DialerRetries
	opt.DialerRetryTimeout = cfg.DialerRetryTimeout
	opt.ReadTimeout = cfg.ReadTimeout
	opt.WriteTimeout = cfg.WriteTimeout
	opt.PoolTimeout = cfg.PoolTimeout
	opt.ContextTimeoutEnabled = cfg.ContextTimeoutEnabled
}

// applyClusterRetryOptions 将命令重试配置映射到 Redis Cluster 客户端选项。
func applyClusterRetryOptions(opt *goredis.ClusterOptions, cfg *RetryConfig) {
	if cfg == nil {
		return
	}
	opt.MaxRetries = cfg.MaxRetries
	opt.MinRetryBackoff = cfg.MinRetryBackoff
	opt.MaxRetryBackoff = cfg.MaxRetryBackoff
}

// applyClusterPoolOptions 将连接池配置映射到 Redis Cluster 客户端选项。
// ClusterClient 的连接池按节点维护，PoolSize 不是整个集群的全局连接上限。
func applyClusterPoolOptions(opt *goredis.ClusterOptions, cfg *PoolConfig) {
	if cfg == nil {
		return
	}
	opt.PoolFIFO = cfg.FIFO
	opt.PoolSize = cfg.Size
	opt.MaxConcurrentDials = cfg.MaxConcurrentDials
	opt.MinIdleConns = cfg.MinIdleConns
	opt.MaxIdleConns = cfg.MaxIdleConns
	opt.MaxActiveConns = cfg.MaxActiveConns
	opt.ConnMaxIdleTime = cfg.ConnMaxIdleTime
	opt.ConnMaxLifetime = cfg.ConnMaxLifetime
	opt.ConnMaxLifetimeJitter = cfg.ConnMaxLifetimeJitter
}

// applyClusterBufferOptions 将读写缓冲区配置映射到 Redis Cluster 客户端选项。
func applyClusterBufferOptions(opt *goredis.ClusterOptions, cfg *BufferConfig) {
	if cfg == nil {
		return
	}
	opt.ReadBufferSize = cfg.ReadBufferSize
	opt.WriteBufferSize = cfg.WriteBufferSize
}

// buildTLSConfig 根据配置构造 tls.Config。
//
// TLS 未启用时返回 nil，让 go-redis 继续使用非 TLS 连接。CAFile 用于追加私有 CA；
// CertFile/KeyFile 用于双向 TLS。文件读取和证书解析错误会在创建客户端时提前返回。
//
// InsecureSkipVerify 只做透传，不在这里做环境判断；是否允许在生产开启应由配置
// 管理或部署策略控制。
func (c *Config) buildTLSConfig() (*tls.Config, error) {
	if c.TLS == nil || !c.TLS.Enabled {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		ServerName:         c.TLS.ServerName,
		InsecureSkipVerify: c.TLS.InsecureSkipVerify,
	}

	if c.TLS.CAFile != "" {
		certPool, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("load system cert pool: %w", err)
		}
		if certPool == nil {
			certPool = x509.NewCertPool()
		}
		caPEM, err := os.ReadFile(c.TLS.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read redis tls ca file: %w", err)
		}
		if ok := certPool.AppendCertsFromPEM(caPEM); !ok {
			return nil, errors.New("redis tls ca file does not contain valid PEM certificates")
		}
		tlsConfig.RootCAs = certPool
	}

	if c.TLS.CertFile != "" && c.TLS.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(c.TLS.CertFile, c.TLS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load redis tls client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// instrumentClient 根据 MonitoringConfig 注册 redisotel instrumentation。
//
// MetricsEnabled 只负责把 Redis 客户端指标接入 OpenTelemetry metrics；Prometheus
// exporter、MeterProvider、HTTP 暴露端口等由应用的观测初始化代码负责配置。
// TracingEnabled 同理，只负责在 Redis 命令执行时创建 span；TraceProvider、
// sampler、exporter 等由应用统一初始化。
func instrumentClient(client Client, cfg *MonitoringConfig) error {
	if cfg == nil {
		return nil
	}
	if cfg.TracingEnabled {
		if err := redisotel.InstrumentTracing(client); err != nil {
			return fmt.Errorf("instrument redis tracing: %w", err)
		}
	}
	if cfg.MetricsEnabled {
		if err := redisotel.InstrumentMetrics(client); err != nil {
			return fmt.Errorf("instrument redis metrics: %w", err)
		}
	}
	return nil
}
