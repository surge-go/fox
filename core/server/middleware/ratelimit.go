package middleware

import (
	"sync"
	"time"

	"github.com/surge-go/fox/core/errors"
	"github.com/surge-go/fox/core/server"
)

const (
	defaultRateLimitRequestsPerSecond = 100
	defaultRateLimitBurst             = 200
	defaultRateLimitCleanupAfter      = time.Minute
	defaultRateLimitStaleAfter        = 5 * time.Minute
)

// RateLimiterConfig 限流器配置
type RateLimiterConfig struct {
	// RequestsPerSecond 每秒允许的请求数
	RequestsPerSecond int
	// Burst 突发流量容量（令牌桶大小）
	Burst int
	// KeyFunc 生成限流键的函数，默认使用客户端 IP
	KeyFunc func(c *server.Context) string
	// OnLimitExceeded 超出限流时的回调，返回 nil 表示已自行处理响应
	OnLimitExceeded func(c *server.Context) error
}

// bucket 令牌桶
type bucket struct {
	tokens    float64
	capacity  float64
	rate      float64
	lastCheck time.Time
	mu        sync.Mutex
}

// allow 检查是否允许请求
func (b *bucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastCheck).Seconds()
	b.lastCheck = now

	// 补充令牌
	b.tokens += elapsed * b.rate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}

	// 消耗一个令牌
	if b.tokens >= 1 {
		b.tokens--
		return true
	}

	return false
}

// rateLimiter 限流器
type rateLimiter struct {
	buckets      map[string]*bucket
	mu           sync.RWMutex
	config       *RateLimiterConfig
	lastCleanup  time.Time
	cleanupAfter time.Duration
	staleAfter   time.Duration
}

// newRateLimiter 创建限流器
func newRateLimiter(cfg *RateLimiterConfig) *rateLimiter {
	return &rateLimiter{
		buckets:      make(map[string]*bucket),
		config:       cfg,
		lastCleanup:  time.Now(),
		cleanupAfter: defaultRateLimitCleanupAfter,
		staleAfter:   defaultRateLimitStaleAfter,
	}
}

// cleanupExpiredIfNeeded 惰性清理长时间未使用的桶，避免每个限流器常驻一个清理协程。
func (rl *rateLimiter) cleanupExpiredIfNeeded(now time.Time) {
	rl.mu.RLock()
	if now.Sub(rl.lastCleanup) < rl.cleanupAfter {
		rl.mu.RUnlock()
		return
	}
	rl.mu.RUnlock()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if now.Sub(rl.lastCleanup) < rl.cleanupAfter {
		return
	}

	rl.lastCleanup = now
	for key, b := range rl.buckets {
		b.mu.Lock()
		stale := now.Sub(b.lastCheck) > rl.staleAfter
		b.mu.Unlock()
		if stale {
			delete(rl.buckets, key)
		}
	}
}

// getBucket 获取或创建令牌桶
func (rl *rateLimiter) getBucket(key string) *bucket {
	rl.cleanupExpiredIfNeeded(time.Now())

	rl.mu.RLock()
	b, exists := rl.buckets[key]
	rl.mu.RUnlock()

	if exists {
		return b
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// 双重检查
	if b, exists := rl.buckets[key]; exists {
		return b
	}

	b = &bucket{
		tokens:    float64(rl.config.Burst),
		capacity:  float64(rl.config.Burst),
		rate:      float64(rl.config.RequestsPerSecond),
		lastCheck: time.Now(),
	}
	rl.buckets[key] = b

	return b
}

// RateLimiter 返回限流中间件
//
// 使用令牌桶算法实现限流，支持突发流量。
//
// 示例：
//
//	// 每秒 100 个请求，突发容量 200
//	srv.Use(middleware.RateLimiter(&middleware.RateLimiterConfig{
//	    RequestsPerSecond: 100,
//	    Burst:             200,
//	}))
//
//	// 按用户 ID 限流
//	srv.Use(middleware.RateLimiter(&middleware.RateLimiterConfig{
//	    RequestsPerSecond: 10,
//	    Burst:             20,
//	    KeyFunc: func(c *server.Context) string {
//	        return c.GetString("user_id")
//	    },
//	}))
func RateLimiter(cfg *RateLimiterConfig) server.HandlerFunc {
	if cfg == nil {
		cfg = &RateLimiterConfig{
			RequestsPerSecond: defaultRateLimitRequestsPerSecond,
			Burst:             defaultRateLimitBurst,
		}
	} else {
		cfgCopy := *cfg
		cfg = &cfgCopy
	}

	if cfg.RequestsPerSecond <= 0 {
		cfg.RequestsPerSecond = defaultRateLimitRequestsPerSecond
	}
	if cfg.Burst <= 0 {
		cfg.Burst = defaultRateLimitBurst
	}

	// 默认使用客户端 IP 作为限流键
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(c *server.Context) string {
			return c.ClientIP()
		}
	}

	// 默认限流响应
	if cfg.OnLimitExceeded == nil {
		cfg.OnLimitExceeded = func(c *server.Context) error {
			return errors.NewWithStatus(4290, 429, "too many requests")
		}
	}

	limiter := newRateLimiter(cfg)

	return func(c *server.Context) {
		key := cfg.KeyFunc(c)
		bucket := limiter.getBucket(key)

		if !bucket.allow() {
			err := cfg.OnLimitExceeded(c)
			if err != nil {
				c.Fail(err)
				return
			}
			c.Abort()
			return
		}

		c.Next()
	}
}
