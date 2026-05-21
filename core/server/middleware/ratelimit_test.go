package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/surge-go/fox/core/server"
)

func TestRateLimiter_Basic(t *testing.T) {
	cfg := &server.Config{Addr: ":8080", Mode: server.ModeTest}
	engine, err := server.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// 每秒 2 个请求，突发容量 2
	engine.Use(RateLimiter(&RateLimiterConfig{
		RequestsPerSecond: 2,
		Burst:             2,
	}))

	engine.GET("/test", func(c *server.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})

	// 前 2 个请求应该成功（消耗突发容量）
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status 200, got %d", i+1, w.Code)
		}
	}

	// 第 3 个请求应该被限流
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	cfg := &server.Config{Addr: ":8080", Mode: server.ModeTest}
	engine, err := server.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// 每秒 10 个请求，突发容量 1
	engine.Use(RateLimiter(&RateLimiterConfig{
		RequestsPerSecond: 10,
		Burst:             1,
	}))

	engine.GET("/test", func(c *server.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})

	// 第 1 个请求成功
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("first request: expected status 200, got %d", w.Code)
	}

	// 第 2 个请求立即发送，应该被限流
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected status 429, got %d", w.Code)
	}

	// 等待 150ms（10 req/s = 100ms/req，加上余量）
	time.Sleep(150 * time.Millisecond)

	// 第 3 个请求应该成功（令牌已补充）
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("third request: expected status 200, got %d", w.Code)
	}
}

func TestRateLimiter_CustomKeyFunc(t *testing.T) {
	cfg := &server.Config{Addr: ":8080", Mode: server.ModeTest}
	engine, err := server.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// 按 User-Agent 限流
	engine.Use(RateLimiter(&RateLimiterConfig{
		RequestsPerSecond: 2,
		Burst:             1,
		KeyFunc: func(c *server.Context) string {
			return c.GetHeader("User-Agent")
		},
	}))

	engine.GET("/test", func(c *server.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})

	// User-Agent: client-1 的第 1 个请求成功
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("User-Agent", "client-1")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("client-1 request 1: expected status 200, got %d", w.Code)
	}

	// User-Agent: client-1 的第 2 个请求被限流
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("User-Agent", "client-1")
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("client-1 request 2: expected status 429, got %d", w.Code)
	}

	// User-Agent: client-2 的第 1 个请求成功（不同的限流键）
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("User-Agent", "client-2")
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("client-2 request 1: expected status 200, got %d", w.Code)
	}
}

func TestRateLimiter_CustomOnLimitExceeded(t *testing.T) {
	cfg := &server.Config{Addr: ":8080", Mode: server.ModeTest}
	engine, err := server.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	customCalled := false
	handlerCalled := false

	engine.Use(RateLimiter(&RateLimiterConfig{
		RequestsPerSecond: 1,
		Burst:             1,
		OnLimitExceeded: func(c *server.Context) error {
			customCalled = true
			c.JSON(http.StatusTooManyRequests, map[string]string{
				"error": "custom rate limit message",
			})
			return nil
		},
	}))

	engine.GET("/test", func(c *server.Context) {
		handlerCalled = true
		c.Ok(map[string]string{"status": "ok"})
	})

	// 第 1 个请求成功
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("first request: expected status 200, got %d", w.Code)
	}

	// 第 2 个请求触发自定义限流响应
	handlerCalled = false
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected status 429, got %d", w.Code)
	}

	if !customCalled {
		t.Error("custom OnLimitExceeded was not called")
	}

	if handlerCalled {
		t.Error("handler should not be called after rate limit is exceeded")
	}

	body := w.Body.String()
	if body != `{"error":"custom rate limit message"}` {
		t.Errorf("unexpected response body: %s", body)
	}
}

func TestRateLimiter_PartialConfigUsesDefaults(t *testing.T) {
	cfg := &server.Config{Addr: ":8080", Mode: server.ModeTest}
	engine, err := server.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	engine.Use(RateLimiter(&RateLimiterConfig{
		KeyFunc: func(c *server.Context) string {
			return "custom-key"
		},
	}))

	engine.GET("/test", func(c *server.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected default rate limit config to allow first request, got %d", w.Code)
	}
}

func TestRateLimiter_CleanupExpiredBuckets(t *testing.T) {
	cfg := &RateLimiterConfig{
		RequestsPerSecond: 1,
		Burst:             1,
	}
	limiter := newRateLimiter(cfg)
	now := time.Now()
	limiter.cleanupAfter = time.Nanosecond
	limiter.staleAfter = time.Minute
	limiter.lastCleanup = now.Add(-time.Minute)
	limiter.buckets["stale"] = &bucket{
		tokens:    1,
		capacity:  1,
		rate:      1,
		lastCheck: now.Add(-2 * time.Minute),
	}

	limiter.getBucket("fresh")

	limiter.mu.RLock()
	_, staleExists := limiter.buckets["stale"]
	_, freshExists := limiter.buckets["fresh"]
	limiter.mu.RUnlock()

	if staleExists {
		t.Error("expected stale bucket to be cleaned up")
	}
	if !freshExists {
		t.Error("expected fresh bucket to be created")
	}
}

func TestRateLimiter_DefaultConfig(t *testing.T) {
	cfg := &server.Config{Addr: ":8080", Mode: server.ModeTest}
	engine, err := server.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// 使用默认配置（nil）
	engine.Use(RateLimiter(nil))

	engine.GET("/test", func(c *server.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})

	// 默认配置应该允许大量请求
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status 200, got %d", i+1, w.Code)
		}
	}
}
