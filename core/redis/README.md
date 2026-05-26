# core/redis

基于 go-redis/v9 的 Redis 客户端，支持 Standalone/Sentinel/Cluster 三种部署模式，提供连接池管理、TLS 加密、链路追踪和指标采集。

## 快速开始

```go
package main

import (
    "context"
    "log"
    "github.com/surge-go/fox/core/redis"
)

func main() {
    // 创建 Redis 客户端
    client, err := redis.NewClient(&redis.Config{
        Mode:  redis.ModeStandalone,
        Addrs: []string{"localhost:6379"},
        Pool: &redis.PoolConfig{
            Size:         100,
            MinIdleConns: 10,
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // 使用 Redis
    ctx := context.Background()
    client.Set(ctx, "key", "value", 0)
    val, _ := client.Get(ctx, "key").Result()
}
```

## 核心功能

### 1. 三种部署模式

#### Standalone（单节点）
```go
client, _ := redis.NewClient(&redis.Config{
    Mode:  redis.ModeStandalone,
    Addrs: []string{"localhost:6379"},
    DB:    0,
})
```

#### Sentinel（哨兵）
```go
client, _ := redis.NewClient(&redis.Config{
    Mode:  redis.ModeSentinel,
    Addrs: []string{
        "sentinel1:26379",
        "sentinel2:26379",
        "sentinel3:26379",
    },
    Sentinel: &redis.SentinelConfig{
        MasterName: "mymaster",
    },
})
```

#### Cluster（集群）
```go
client, _ := redis.NewClient(&redis.Config{
    Mode:  redis.ModeCluster,
    Addrs: []string{
        "node1:6379",
        "node2:6379",
        "node3:6379",
    },
})
```

### 2. 连接池管理
```go
client, _ := redis.NewClient(&redis.Config{
    Mode:  redis.ModeStandalone,
    Addrs: []string{"localhost:6379"},
    Pool: &redis.PoolConfig{
        Size:            100,              // 连接池大小
        MinIdleConns:    10,               // 最小空闲连接
        MaxIdleConns:    50,               // 最大空闲连接
        MaxActiveConns:  200,              // 最大活跃连接
        ConnMaxIdleTime: 10 * time.Minute, // 连接最大空闲时间
        ConnMaxLifetime: 1 * time.Hour,    // 连接最大复用时间
    },
})
```

### 3. 超时配置
```go
client, _ := redis.NewClient(&redis.Config{
    Mode:  redis.ModeStandalone,
    Addrs: []string{"localhost:6379"},
    Timeout: &redis.TimeoutConfig{
        DialTimeout:  5 * time.Second,
        ReadTimeout:  3 * time.Second,
        WriteTimeout: 3 * time.Second,
        PoolTimeout:  4 * time.Second,
    },
})
```

### 4. 重试策略
```go
client, _ := redis.NewClient(&redis.Config{
    Mode:  redis.ModeStandalone,
    Addrs: []string{"localhost:6379"},
    Retry: &redis.RetryConfig{
        MaxRetries:      3,
        MinRetryBackoff: 8 * time.Millisecond,
        MaxRetryBackoff: 512 * time.Millisecond,
    },
})
```

### 5. TLS 加密
```go
client, _ := redis.NewClient(&redis.Config{
    Mode:  redis.ModeStandalone,
    Addrs: []string{"localhost:6379"},
    TLS: &redis.TLSConfig{
        Enabled:    true,
        ServerName: "redis.example.com",
        CAFile:     "/path/to/ca.crt",
        CertFile:   "/path/to/client.crt",
        KeyFile:    "/path/to/client.key",
    },
})
```

### 6. 链路追踪 + 指标采集
```go
client, _ := redis.NewClient(&redis.Config{
    Mode:  redis.ModeStandalone,
    Addrs: []string{"localhost:6379"},
    Monitoring: &redis.MonitoringConfig{
        TracingEnabled: true,  // OpenTelemetry 链路追踪
        MetricsEnabled: true,  // Prometheus 指标采集
    },
})
```

### 7. 认证
```go
client, _ := redis.NewClient(&redis.Config{
    Mode:     redis.ModeStandalone,
    Addrs:    []string{"localhost:6379"},
    Username: "default",  // Redis 6.0+ ACL
    Password: "your-password",
})
```

## 配置参考

### 完整配置示例

```go
cfg := &redis.Config{
    // 基础配置
    Mode:     redis.ModeStandalone,
    Addrs:    []string{"localhost:6379"},
    Network:  "tcp",
    DB:       0,
    Username: "default",
    Password: "password",
    
    // 客户端标识
    ClientName:     "user-service",
    IdentitySuffix: "prod-01",
    
    // 协议配置
    Protocol: 3,  // RESP3
    
    // 超时配置
    Timeout: &redis.TimeoutConfig{
        DialTimeout:           5 * time.Second,
        ReadTimeout:           3 * time.Second,
        WriteTimeout:          3 * time.Second,
        PoolTimeout:           4 * time.Second,
        ContextTimeoutEnabled: true,
    },
    
    // 重试配置
    Retry: &redis.RetryConfig{
        MaxRetries:      3,
        MinRetryBackoff: 8 * time.Millisecond,
        MaxRetryBackoff: 512 * time.Millisecond,
    },
    
    // 连接池配置
    Pool: &redis.PoolConfig{
        FIFO:               false,
        Size:               100,
        MaxConcurrentDials: 10,
        MinIdleConns:       10,
        MaxIdleConns:       50,
        MaxActiveConns:     200,
        ConnMaxIdleTime:    10 * time.Minute,
        ConnMaxLifetime:    1 * time.Hour,
    },
    
    // TLS 配置
    TLS: &redis.TLSConfig{
        Enabled:    true,
        ServerName: "redis.example.com",
    },
    
    // 监控配置
    Monitoring: &redis.MonitoringConfig{
        TracingEnabled: true,
        MetricsEnabled: true,
    },
}

client, err := redis.NewClient(cfg)
```

### 配置字段说明

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `Mode` | `Mode` | `standalone` | 部署模式：standalone/sentinel/cluster |
| `Addrs` | `[]string` | - | Redis 节点地址列表 |
| `Network` | `string` | `tcp` | 网络类型：tcp/tcp4/tcp6/unix |
| `DB` | `int` | `0` | 数据库编号（仅 standalone/sentinel） |
| `Username` | `string` | - | 用户名（Redis 6.0+ ACL） |
| `Password` | `string` | - | 密码 |
| `ClientName` | `string` | - | 客户端名称（便于排查连接来源） |
| `Protocol` | `int` | `3` | RESP 协议版本：2/3 |

## YAML 配置示例

```yaml
redis:
  mode: standalone
  addrs:
    - "localhost:6379"
  db: 0
  username: default
  password: your-password
  client_name: user-service
  
  timeout:
    dial_timeout: 5s
    read_timeout: 3s
    write_timeout: 3s
    pool_timeout: 4s
    context_timeout_enabled: true
  
  retry:
    max_retries: 3
    min_retry_backoff: 8ms
    max_retry_backoff: 512ms
  
  pool:
    size: 100
    min_idle_conns: 10
    max_idle_conns: 50
    max_active_conns: 200
    conn_max_idle_time: 10m
    conn_max_lifetime: 1h
  
  monitoring:
    tracing_enabled: true
    metrics_enabled: true
```

## 最佳实践

### 1. 生产环境配置
```go
client, _ := redis.NewClient(&redis.Config{
    Mode:     redis.ModeCluster,  // 生产环境推荐集群模式
    Addrs:    []string{"node1:6379", "node2:6379", "node3:6379"},
    Password: os.Getenv("REDIS_PASSWORD"),
    ClientName: "user-service",
    
    Timeout: &redis.TimeoutConfig{
        DialTimeout:           5 * time.Second,
        ReadTimeout:           3 * time.Second,
        WriteTimeout:          3 * time.Second,
        PoolTimeout:           4 * time.Second,
        ContextTimeoutEnabled: true,
    },
    
    Retry: &redis.RetryConfig{
        MaxRetries:      3,
        MinRetryBackoff: 8 * time.Millisecond,
        MaxRetryBackoff: 512 * time.Millisecond,
    },
    
    Pool: &redis.PoolConfig{
        Size:            100,
        MinIdleConns:    10,
        MaxActiveConns:  200,
        ConnMaxIdleTime: 10 * time.Minute,
        ConnMaxLifetime: 1 * time.Hour,
    },
    
    Monitoring: &redis.MonitoringConfig{
        TracingEnabled: true,
        MetricsEnabled: true,
    },
})
```

### 2. 开发环境配置
```go
client, _ := redis.NewClient(&redis.Config{
    Mode:  redis.ModeStandalone,
    Addrs: []string{"localhost:6379"},
    Pool: &redis.PoolConfig{
        Size:         10,
        MinIdleConns: 2,
    },
})
```

### 3. Pipeline 批量操作
```go
pipe := client.Pipeline()
pipe.Set(ctx, "key1", "value1", 0)
pipe.Set(ctx, "key2", "value2", 0)
pipe.Incr(ctx, "counter")
_, err := pipe.Exec(ctx)
```

### 4. 分布式锁
```go
// 获取锁
ok, err := client.SetNX(ctx, "lock:resource", "owner", 10*time.Second).Result()
if !ok {
    // 锁已被占用
}

// 释放锁（使用 Lua 脚本保证原子性）
script := `
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
else
    return 0
end
`
client.Eval(ctx, script, []string{"lock:resource"}, "owner")
```

### 5. 缓存模式
```go
// Cache-Aside 模式
func GetUser(ctx context.Context, id int) (*User, error) {
    // 1. 尝试从缓存读取
    key := fmt.Sprintf("user:%d", id)
    val, err := client.Get(ctx, key).Result()
    if err == nil {
        var user User
        json.Unmarshal([]byte(val), &user)
        return &user, nil
    }
    
    // 2. 缓存未命中，从数据库读取
    user, err := db.GetUser(id)
    if err != nil {
        return nil, err
    }
    
    // 3. 写入缓存
    data, _ := json.Marshal(user)
    client.Set(ctx, key, data, 1*time.Hour)
    
    return user, nil
}
```

### 6. Pub/Sub 消息订阅
```go
// 订阅
pubsub := client.Subscribe(ctx, "channel")
defer pubsub.Close()

ch := pubsub.Channel()
for msg := range ch {
    fmt.Println(msg.Channel, msg.Payload)
}

// 发布
client.Publish(ctx, "channel", "message")
```

### 7. 连接池监控
```go
stats := client.PoolStats()
fmt.Printf("总连接数: %d\n", stats.TotalConns)
fmt.Printf("空闲连接: %d\n", stats.IdleConns)
fmt.Printf("过期连接: %d\n", stats.StaleConns)
```

## 常用命令

### 字符串操作
```go
client.Set(ctx, "key", "value", 0)
client.Get(ctx, "key")
client.Incr(ctx, "counter")
client.Expire(ctx, "key", 1*time.Hour)
```

### 哈希操作
```go
client.HSet(ctx, "user:1", "name", "alice")
client.HGet(ctx, "user:1", "name")
client.HGetAll(ctx, "user:1")
```

### 列表操作
```go
client.LPush(ctx, "queue", "task1")
client.RPop(ctx, "queue")
client.LRange(ctx, "queue", 0, -1)
```

### 集合操作
```go
client.SAdd(ctx, "tags", "go", "redis")
client.SMembers(ctx, "tags")
client.SIsMember(ctx, "tags", "go")
```

### 有序集合操作
```go
client.ZAdd(ctx, "leaderboard", redis.Z{Score: 100, Member: "alice"})
client.ZRevRange(ctx, "leaderboard", 0, 9)  // Top 10
client.ZScore(ctx, "leaderboard", "alice")
```

## 性能指标

### Prometheus 指标
启用 `MetricsEnabled` 后自动采集：
- `redis_pool_hits_total` - 连接池命中次数
- `redis_pool_misses_total` - 连接池未命中次数
- `redis_pool_timeouts_total` - 连接池超时次数
- `redis_pool_conns` - 当前连接数
- `redis_pool_idle_conns` - 空闲连接数
- `redis_pool_stale_conns` - 过期连接数

### 性能优化
```go
// 1. 使用 Pipeline 批量操作
pipe := client.Pipeline()
for i := 0; i < 1000; i++ {
    pipe.Set(ctx, fmt.Sprintf("key%d", i), i, 0)
}
pipe.Exec(ctx)

// 2. 使用连接池
cfg.Pool.Size = 100  // 根据 QPS 调整

// 3. 启用 prepared statement（RESP3）
cfg.Protocol = 3

// 4. 合理设置超时
cfg.Timeout.ReadTimeout = 3 * time.Second
```

## 注意事项

1. **连接池大小**：根据 QPS 和命令耗时合理设置，避免过大或过小
2. **超时配置**：设置合理的超时时间，避免慢命令阻塞
3. **重试策略**：非幂等命令谨慎重试
4. **Cluster 模式**：不支持多 DB，DB 必须为 0
5. **Pipeline**：批量操作使用 Pipeline 提升性能
6. **大 Key**：避免存储过大的 value（建议 < 10KB）

## 依赖

- `github.com/redis/go-redis/v9` - Redis 客户端
- `github.com/redis/go-redis/extra/redisotel/v9` - OpenTelemetry 集成

## 相关文档

- [go-redis 官方文档](https://redis.uptrace.dev/)
- [Redis 官方文档](https://redis.io/docs/)
- [Redis 最佳实践](https://redis.io/docs/manual/patterns/)
