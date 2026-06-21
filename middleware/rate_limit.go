package middleware

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/surge-go/fox"
)

const (
	defaultRateLimitLimit           = 60
	defaultRateLimitWindow          = time.Minute
	defaultRateLimitCleanupInterval = time.Minute
	defaultMemoryRateLimitShards    = 32
	defaultRedisRateLimitKeyPrefix  = "fox:rate_limit"

	rateLimitHeaderLimit      = "X-RateLimit-Limit"
	rateLimitHeaderRemaining  = "X-RateLimit-Remaining"
	rateLimitHeaderReset      = "X-RateLimit-Reset"
	rateLimitHeaderRetryAfter = "Retry-After"
)

// RateLimitKeyFunc 根据请求生成限流键。
type RateLimitKeyFunc func(*fox.Context) string

// RateLimitDenyHandler 处理被限流拒绝的请求。
type RateLimitDenyHandler func(*fox.Context, RateLimitResult)

// RateLimitErrorHandler 处理限流存储执行失败的请求。
type RateLimitErrorHandler func(*fox.Context, error)

// RateLimitStore 表示限流状态存储。
type RateLimitStore interface {
	Allow(ctx context.Context, key string, now time.Time) (RateLimitResult, error)
}

// RedisRateLimitClient 表示 Redis 限流存储需要的最小客户端能力。
type RedisRateLimitClient interface {
	goredis.Scripter
}

// RateLimitResult 表示一次限流判定结果。
type RateLimitResult struct {
	Allowed    bool
	Limit      int
	Remaining  int
	Reset      time.Time
	RetryAfter time.Duration
}

// RateLimitConfig 表示限流中间件配置。
type RateLimitConfig struct {
	// Limit 表示 Window 时间内生成的令牌数，0 使用默认值。
	Limit int
	// Window 表示限流窗口，0 使用默认值。
	Window time.Duration
	// Burst 表示允许的最大突发请求数，0 使用 Limit。
	Burst int
	// KeyFunc 根据请求生成限流键，nil 使用客户端 IP。
	KeyFunc RateLimitKeyFunc
	// DenyHandler 处理被拒绝的请求，nil 使用默认 429 JSON 响应。
	DenyHandler RateLimitDenyHandler
	// ErrorHandler 处理限流存储错误，nil 使用默认 503 JSON 响应。
	ErrorHandler RateLimitErrorHandler
	// Redis 表示 Redis 客户端实例；Store 为 nil 且设置 Redis 时使用 Redis 令牌桶存储。
	Redis RedisRateLimitClient
	// Store 保存限流状态，非 nil 时优先于 Redis；nil 使用本地内存令牌桶。
	Store RateLimitStore
	// RedisKeyPrefix 表示 Redis 限流键前缀，空值使用默认前缀。
	RedisKeyPrefix string
	// CleanupInterval 表示本地内存状态清理间隔，0 使用默认值。
	CleanupInterval time.Duration
}

// DefaultRateLimitConfig 返回限流中间件默认配置。
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Limit:           defaultRateLimitLimit,
		Window:          defaultRateLimitWindow,
		Burst:           defaultRateLimitLimit,
		KeyFunc:         ClientIPRateLimitKey,
		RedisKeyPrefix:  defaultRedisRateLimitKeyPrefix,
		CleanupInterval: defaultRateLimitCleanupInterval,
	}
}

// RateLimit 返回使用默认配置的限流中间件。
func RateLimit() fox.HandlerFunc {
	return RateLimitWithConfig(DefaultRateLimitConfig())
}

// RateLimitWithConfig 返回使用自定义配置的限流中间件。
func RateLimitWithConfig(cfg RateLimitConfig) fox.HandlerFunc {
	cfg = normalizeRateLimitConfig(cfg)
	return func(c *fox.Context) {
		key := rateLimitKey(c, cfg.KeyFunc)

		result, err := cfg.Store.Allow(c.StdContext(), key, time.Now())
		if err != nil {
			cfg.ErrorHandler(c, err)
			return
		}
		setRateLimitHeaders(c, result)
		if !result.Allowed {
			cfg.DenyHandler(c, result)
			return
		}
		c.Next()
	}
}

// ClientIPRateLimitKey 使用客户端 IP 作为限流键。
func ClientIPRateLimitKey(c *fox.Context) string {
	return c.ClientIP()
}

// GlobalRateLimitKey 让所有请求共享同一个限流键。
func GlobalRateLimitKey(*fox.Context) string {
	return "global"
}

func rateLimitKey(c *fox.Context, keyFunc RateLimitKeyFunc) string {
	if keyFunc != nil {
		if key := keyFunc(c); key != "" {
			return key
		}
	}
	if key := ClientIPRateLimitKey(c); key != "" {
		return key
	}
	return "global"
}

func normalizeRateLimitConfig(cfg RateLimitConfig) RateLimitConfig {
	defaults := DefaultRateLimitConfig()
	if cfg.Limit <= 0 {
		cfg.Limit = defaults.Limit
	}
	if cfg.Window <= 0 {
		cfg.Window = defaults.Window
	}
	if cfg.Burst <= 0 {
		cfg.Burst = cfg.Limit
	}
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = defaults.KeyFunc
	}
	if cfg.DenyHandler == nil {
		cfg.DenyHandler = defaultRateLimitDenyHandler
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = defaultRateLimitErrorHandler
	}
	if cfg.RedisKeyPrefix == "" {
		cfg.RedisKeyPrefix = defaults.RedisKeyPrefix
	}
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = defaults.CleanupInterval
	}
	if cfg.Store == nil {
		if cfg.Redis != nil {
			cfg.Store = NewRedisRateLimitStore(cfg.Redis, RedisRateLimitStoreConfig{
				Limit:     cfg.Limit,
				Window:    cfg.Window,
				Burst:     cfg.Burst,
				KeyPrefix: cfg.RedisKeyPrefix,
			})
		} else {
			cfg.Store = NewMemoryRateLimitStore(MemoryRateLimitStoreConfig{
				Limit:           cfg.Limit,
				Window:          cfg.Window,
				Burst:           cfg.Burst,
				CleanupInterval: cfg.CleanupInterval,
			})
		}
	}
	return cfg
}

func defaultRateLimitDenyHandler(c *fox.Context, result RateLimitResult) {
	if result.RetryAfter > 0 {
		c.SetHeader(rateLimitHeaderRetryAfter, strconv.FormatInt(int64(math.Ceil(result.RetryAfter.Seconds())), 10))
	}
	c.Fail(c.Errors().ErrTooManyRequests())
}

func defaultRateLimitErrorHandler(c *fox.Context, _ error) {
	c.Fail(c.Errors().ErrServiceUnavailable())
}

func setRateLimitHeaders(c *fox.Context, result RateLimitResult) {
	c.SetHeader(rateLimitHeaderLimit, strconv.Itoa(result.Limit))
	c.SetHeader(rateLimitHeaderRemaining, strconv.Itoa(result.Remaining))
	if !result.Reset.IsZero() {
		c.SetHeader(rateLimitHeaderReset, strconv.FormatInt(result.Reset.Unix(), 10))
	}
}

// MemoryRateLimitStoreConfig 表示本地内存令牌桶存储配置。
type MemoryRateLimitStoreConfig struct {
	// Limit 表示 Window 时间内生成的令牌数。
	Limit int
	// Window 表示限流窗口。
	Window time.Duration
	// Burst 表示允许的最大突发请求数。
	Burst int
	// CleanupInterval 表示内存状态清理间隔。
	CleanupInterval time.Duration
}

// NewMemoryRateLimitStore 创建本地内存令牌桶限流存储。
func NewMemoryRateLimitStore(cfg MemoryRateLimitStoreConfig) RateLimitStore {
	if cfg.Limit <= 0 {
		cfg.Limit = defaultRateLimitLimit
	}
	if cfg.Window <= 0 {
		cfg.Window = defaultRateLimitWindow
	}
	if cfg.Burst <= 0 {
		cfg.Burst = cfg.Limit
	}
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = defaultRateLimitCleanupInterval
	}
	return &memoryRateLimitStore{
		limit:           cfg.Limit,
		window:          cfg.Window,
		burst:           cfg.Burst,
		cleanupInterval: cfg.CleanupInterval,
		shards:          newMemoryRateLimitShards(defaultMemoryRateLimitShards),
	}
}

type memoryRateLimitStore struct {
	limit           int
	window          time.Duration
	burst           int
	cleanupInterval time.Duration
	shards          []memoryRateLimitShard
}

type memoryRateLimitShard struct {
	mu          sync.Mutex
	lastCleanup time.Time
	buckets     map[string]*memoryRateLimitBucket
}

type memoryRateLimitBucket struct {
	tokens float64
	last   time.Time
}

func (s *memoryRateLimitStore) Allow(_ context.Context, key string, now time.Time) (RateLimitResult, error) {
	shard := s.shard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if shard.lastCleanup.IsZero() {
		shard.lastCleanup = now
	}
	if now.Sub(shard.lastCleanup) >= s.cleanupInterval {
		s.cleanup(shard, now)
	}

	bucket := shard.buckets[key]
	if bucket == nil {
		bucket = &memoryRateLimitBucket{
			tokens: float64(s.burst),
			last:   now,
		}
		shard.buckets[key] = bucket
	}

	s.refill(bucket, now)
	allowed := bucket.tokens >= 1
	if allowed {
		bucket.tokens--
	}

	result := RateLimitResult{
		Allowed:   allowed,
		Limit:     s.limit,
		Remaining: int(math.Floor(bucket.tokens)),
	}
	if result.Remaining < 0 {
		result.Remaining = 0
	}
	result.RetryAfter = s.retryAfter(bucket.tokens)
	if allowed {
		result.Reset = now.Add(s.fullRefillAfter(bucket.tokens))
	} else {
		result.Reset = now.Add(result.RetryAfter)
	}
	return result, nil
}

func (s *memoryRateLimitStore) shard(key string) *memoryRateLimitShard {
	if len(s.shards) == 1 {
		return &s.shards[0]
	}
	return &s.shards[int(fnv32a(key)%uint32(len(s.shards)))]
}

func (s *memoryRateLimitStore) refill(bucket *memoryRateLimitBucket, now time.Time) {
	if now.Before(bucket.last) {
		bucket.last = now
		return
	}
	elapsed := now.Sub(bucket.last)
	if elapsed <= 0 {
		return
	}
	bucket.tokens += elapsed.Seconds() * s.tokensPerSecond()
	if bucket.tokens > float64(s.burst) {
		bucket.tokens = float64(s.burst)
	}
	bucket.last = now
}

func (s *memoryRateLimitStore) cleanup(shard *memoryRateLimitShard, now time.Time) {
	ttl := maxDuration(s.cleanupInterval, s.fullRefillAfter(0))
	for key, bucket := range shard.buckets {
		if now.Sub(bucket.last) >= ttl {
			delete(shard.buckets, key)
		}
	}
	shard.lastCleanup = now
}

func (s *memoryRateLimitStore) retryAfter(tokens float64) time.Duration {
	if tokens >= 1 {
		return 0
	}
	seconds := (1 - tokens) / s.tokensPerSecond()
	return time.Duration(math.Ceil(seconds * float64(time.Second)))
}

func (s *memoryRateLimitStore) fullRefillAfter(tokens float64) time.Duration {
	missing := float64(s.burst) - tokens
	if missing <= 0 {
		return 0
	}
	seconds := missing / s.tokensPerSecond()
	return time.Duration(math.Ceil(seconds * float64(time.Second)))
}

func (s *memoryRateLimitStore) tokensPerSecond() float64 {
	return float64(s.limit) / s.window.Seconds()
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func newMemoryRateLimitShards(count int) []memoryRateLimitShard {
	if count <= 0 {
		count = 1
	}
	shards := make([]memoryRateLimitShard, count)
	for i := range shards {
		shards[i].buckets = make(map[string]*memoryRateLimitBucket)
	}
	return shards
}

func fnv32a(value string) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	hash := uint32(offset32)
	for i := 0; i < len(value); i++ {
		hash ^= uint32(value[i])
		hash *= prime32
	}
	return hash
}

// RedisRateLimitStoreConfig 表示 Redis 令牌桶存储配置。
type RedisRateLimitStoreConfig struct {
	// Limit 表示 Window 时间内生成的令牌数。
	Limit int
	// Window 表示限流窗口。
	Window time.Duration
	// Burst 表示允许的最大突发请求数。
	Burst int
	// KeyPrefix 表示 Redis 限流键前缀。
	KeyPrefix string
}

// NewRedisRateLimitStore 创建 Redis 令牌桶限流存储。
func NewRedisRateLimitStore(client RedisRateLimitClient, cfg RedisRateLimitStoreConfig) RateLimitStore {
	if cfg.Limit <= 0 {
		cfg.Limit = defaultRateLimitLimit
	}
	if cfg.Window <= 0 {
		cfg.Window = defaultRateLimitWindow
	}
	if cfg.Burst <= 0 {
		cfg.Burst = cfg.Limit
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = defaultRedisRateLimitKeyPrefix
	}
	return &redisRateLimitStore{
		client:    client,
		script:    redisRateLimitLua,
		limit:     cfg.Limit,
		window:    cfg.Window,
		burst:     cfg.Burst,
		keyPrefix: cfg.KeyPrefix,
		ttl:       redisRateLimitTTL(cfg.Limit, cfg.Window, cfg.Burst),
	}
}

type redisRateLimitStore struct {
	client    RedisRateLimitClient
	script    *goredis.Script
	limit     int
	window    time.Duration
	burst     int
	keyPrefix string
	ttl       time.Duration
}

func (s *redisRateLimitStore) Allow(ctx context.Context, key string, now time.Time) (RateLimitResult, error) {
	if s.client == nil {
		return RateLimitResult{}, errors.New("fox middleware: nil redis rate limit client")
	}

	values, err := s.script.Run(
		ctx,
		s.client,
		[]string{s.redisKey(key)},
		s.limit,
		s.burst,
		durationMillisAtLeastOne(s.window),
		now.UnixMilli(),
		durationMillisAtLeastOne(s.ttl),
	).Result()
	if err != nil {
		return RateLimitResult{}, err
	}
	return parseRedisRateLimitResult(values)
}

func (s *redisRateLimitStore) redisKey(key string) string {
	return s.keyPrefix + ":" + key
}

func redisRateLimitTTL(limit int, window time.Duration, burst int) time.Duration {
	if limit <= 0 || window <= 0 || burst <= 0 {
		return 2 * defaultRateLimitWindow
	}
	fullRefill := time.Duration(math.Ceil(float64(burst) / float64(limit) * float64(window)))
	return 2 * maxDuration(window, fullRefill)
}

func durationMillisAtLeastOne(d time.Duration) int64 {
	milliseconds := d.Milliseconds()
	if milliseconds < 1 {
		return 1
	}
	return milliseconds
}

func parseRedisRateLimitResult(values any) (RateLimitResult, error) {
	items, ok := values.([]any)
	if !ok {
		if values, ok := values.([]interface{}); ok {
			items = values
		}
	}
	if len(items) != 5 {
		return RateLimitResult{}, fmt.Errorf("fox middleware: invalid redis rate limit result %T", values)
	}

	allowed, err := redisInt64(items[0])
	if err != nil {
		return RateLimitResult{}, err
	}
	limit, err := redisInt64(items[1])
	if err != nil {
		return RateLimitResult{}, err
	}
	remaining, err := redisInt64(items[2])
	if err != nil {
		return RateLimitResult{}, err
	}
	reset, err := redisInt64(items[3])
	if err != nil {
		return RateLimitResult{}, err
	}
	retryAfter, err := redisInt64(items[4])
	if err != nil {
		return RateLimitResult{}, err
	}

	return RateLimitResult{
		Allowed:    allowed == 1,
		Limit:      int(limit),
		Remaining:  int(remaining),
		Reset:      time.UnixMilli(reset),
		RetryAfter: time.Duration(retryAfter) * time.Millisecond,
	}, nil
}

func redisInt64(value any) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case uint:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case uint64:
		if v > math.MaxInt64 {
			return 0, fmt.Errorf("fox middleware: redis integer overflows int64: %d", v)
		}
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	case []byte:
		return strconv.ParseInt(string(v), 10, 64)
	default:
		return 0, fmt.Errorf("fox middleware: invalid redis integer %T", value)
	}
}

var redisRateLimitLua = goredis.NewScript(redisRateLimitScript)

const redisRateLimitScript = `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local window_ms = tonumber(ARGV[3])
local now_ms = tonumber(ARGV[4])
local ttl_ms = tonumber(ARGV[5])

local rate = limit / window_ms
local bucket = redis.call("HMGET", key, "tokens", "last")
local tokens = tonumber(bucket[1])
local last = tonumber(bucket[2])

if tokens == nil then
	tokens = burst
	last = now_ms
end

if now_ms < last then
	last = now_ms
end

local elapsed = math.max(0, now_ms - last)
tokens = math.min(burst, tokens + elapsed * rate)

local allowed = 0
if tokens >= 1 then
	allowed = 1
	tokens = tokens - 1
end

local remaining = math.max(0, math.floor(tokens))
local retry_after_ms = 0
if allowed == 0 then
	retry_after_ms = math.ceil((1 - tokens) / rate)
end

local reset_after_ms = retry_after_ms
if allowed == 1 then
	reset_after_ms = math.ceil((burst - tokens) / rate)
end
redis.call("HMSET", key, "tokens", tokens, "last", now_ms)
redis.call("PEXPIRE", key, ttl_ms)

return { allowed, limit, remaining, now_ms + reset_after_ms, retry_after_ms }
`
