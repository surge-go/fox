package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/surge-go/fox"
)

func newRateLimitTestEngine(handler fox.HandlerFunc) *fox.Engine {
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(handler)
	e.GET("/test", func(c *fox.Context) {
		c.Ok(map[string]string{"status": "ok"})
	})
	return e
}

func TestRateLimitAllowsBurstAndRejectsExcess(t *testing.T) {
	e := newRateLimitTestEngine(RateLimitWithConfig(RateLimitConfig{
		Limit:  2,
		Window: time.Minute,
		Burst:  2,
		KeyFunc: func(*fox.Context) string {
			return "client-1"
		},
	}))

	for i := 0; i < 2; i++ {
		rec := performRateLimitRequest(e, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d", i+1, rec.Code, http.StatusOK)
		}
		if got := rec.Header().Get(rateLimitHeaderLimit); got != "2" {
			t.Fatalf("X-RateLimit-Limit = %q, want 2", got)
		}
	}

	rec := performRateLimitRequest(e, "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if got := rec.Header().Get(rateLimitHeaderRemaining); got != "0" {
		t.Fatalf("X-RateLimit-Remaining = %q, want 0", got)
	}
	if got := rec.Header().Get(rateLimitHeaderRetryAfter); got == "" {
		t.Fatal("Retry-After is empty")
	}

	var resp fox.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Code != http.StatusTooManyRequests || resp.Message != "too many requests" {
		t.Fatalf("response = %+v, want 429 too many requests", resp)
	}
}

func TestRateLimitKeyFuncIsolatesClients(t *testing.T) {
	e := newRateLimitTestEngine(RateLimitWithConfig(RateLimitConfig{
		Limit:  1,
		Window: time.Minute,
		Burst:  1,
		KeyFunc: func(c *fox.Context) string {
			return c.GetHeader("X-Client-ID")
		},
	}))

	if rec := performRateLimitRequest(e, "a"); rec.Code != http.StatusOK {
		t.Fatalf("client a first status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec := performRateLimitRequest(e, "b"); rec.Code != http.StatusOK {
		t.Fatalf("client b first status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec := performRateLimitRequest(e, "a"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("client a second status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitEmptyKeyFallsBackToClientIP(t *testing.T) {
	e := newRateLimitTestEngine(RateLimitWithConfig(RateLimitConfig{
		Limit:  1,
		Window: time.Minute,
		Burst:  1,
		KeyFunc: func(*fox.Context) string {
			return ""
		},
	}))

	if rec := performRateLimitRequestFrom(e, "198.51.100.1:1234"); rec.Code != http.StatusOK {
		t.Fatalf("first client status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec := performRateLimitRequestFrom(e, "198.51.100.2:1234"); rec.Code != http.StatusOK {
		t.Fatalf("second client status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec := performRateLimitRequestFrom(e, "198.51.100.1:1234"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("first client second status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitCustomDenyHandler(t *testing.T) {
	e := newRateLimitTestEngine(RateLimitWithConfig(RateLimitConfig{
		Limit:  1,
		Window: time.Minute,
		Burst:  1,
		KeyFunc: func(*fox.Context) string {
			return "client-1"
		},
		DenyHandler: func(c *fox.Context, result RateLimitResult) {
			c.SetHeader("X-Deny-Remaining", strconv.Itoa(result.Remaining))
			c.AbortWithStatusJSON(http.StatusTeapot, map[string]string{"message": "slow down"})
		},
	}))

	if rec := performRateLimitRequest(e, ""); rec.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d", rec.Code, http.StatusOK)
	}
	rec := performRateLimitRequest(e, "")
	if rec.Code != http.StatusTeapot {
		t.Fatalf("second status = %d, want %d", rec.Code, http.StatusTeapot)
	}
	if got := rec.Header().Get("X-Deny-Remaining"); got != "0" {
		t.Fatalf("X-Deny-Remaining = %q, want 0", got)
	}
}

func TestMemoryRateLimitStoreRefillsTokens(t *testing.T) {
	store := NewMemoryRateLimitStore(MemoryRateLimitStoreConfig{
		Limit:  2,
		Window: time.Second,
		Burst:  1,
	})
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)

	if result, err := store.Allow(context.Background(), "client-1", now); err != nil || !result.Allowed {
		t.Fatalf("first result = %+v, want allowed", result)
	}
	if result, err := store.Allow(context.Background(), "client-1", now); err != nil || result.Allowed {
		t.Fatalf("second result = %+v, want rejected", result)
	}
	if result, err := store.Allow(context.Background(), "client-1", now.Add(500*time.Millisecond)); err != nil || !result.Allowed {
		t.Fatalf("refilled result = %+v, want allowed", result)
	}
}

func TestRateLimitUsesRedisClient(t *testing.T) {
	client := &fakeRedisRateLimitClient{
		result: []any{int64(1), int64(3), int64(2), time.Now().Add(time.Minute).UnixMilli(), int64(0)},
	}
	e := newRateLimitTestEngine(RateLimitWithConfig(RateLimitConfig{
		Limit:  3,
		Window: time.Minute,
		Burst:  3,
		Redis:  client,
		KeyFunc: func(*fox.Context) string {
			return "client-1"
		},
	}))

	rec := performRateLimitRequest(e, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get(rateLimitHeaderRemaining); got != "2" {
		t.Fatalf("X-RateLimit-Remaining = %q, want 2", got)
	}
	if len(client.keys) != 1 || client.keys[0] != defaultRedisRateLimitKeyPrefix+":client-1" {
		t.Fatalf("redis keys = %#v, want prefixed client key", client.keys)
	}
	if len(client.args) != 5 || client.args[0] != 3 || client.args[1] != 3 {
		t.Fatalf("redis args = %#v, want limit and burst", client.args)
	}
	if client.evalShaCalls != 1 || client.evalCalls != 0 {
		t.Fatalf("redis calls EvalSha=%d Eval=%d, want EvalSha only", client.evalShaCalls, client.evalCalls)
	}
}

func TestRedisRateLimitRejectedResetMatchesRetryAfter(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	retryAfter := 750 * time.Millisecond
	store := NewRedisRateLimitStore(&fakeRedisRateLimitClient{
		result: []any{
			int64(0),
			int64(2),
			int64(0),
			now.Add(retryAfter).UnixMilli(),
			retryAfter.Milliseconds(),
		},
	}, RedisRateLimitStoreConfig{
		Limit:  2,
		Window: time.Second,
		Burst:  2,
	})

	result, err := store.Allow(context.Background(), "client-1", now)
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if result.Allowed {
		t.Fatalf("Allowed = true, want false")
	}
	if result.RetryAfter != retryAfter {
		t.Fatalf("RetryAfter = %s, want %s", result.RetryAfter, retryAfter)
	}
	if !result.Reset.Equal(now.Add(retryAfter)) {
		t.Fatalf("Reset = %s, want %s", result.Reset, now.Add(retryAfter))
	}
}

func TestRateLimitRedisErrorUsesErrorHandler(t *testing.T) {
	client := &fakeRedisRateLimitClient{err: errors.New("redis down")}
	e := newRateLimitTestEngine(RateLimitWithConfig(RateLimitConfig{
		Redis: client,
		ErrorHandler: func(c *fox.Context, err error) {
			c.SetHeader("X-RateLimit-Error", err.Error())
			c.AbortWithStatusJSON(http.StatusAccepted, map[string]string{"message": "open"})
		},
	}))

	rec := performRateLimitRequest(e, "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if got := rec.Header().Get("X-RateLimit-Error"); got != "redis down" {
		t.Fatalf("X-RateLimit-Error = %q, want redis down", got)
	}
}

func performRateLimitRequest(e *fox.Engine, clientID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	if clientID != "" {
		req.Header.Set("X-Client-ID", clientID)
	}
	return performRateLimitRawRequest(e, req)
}

func performRateLimitRequestFrom(e *fox.Engine, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = remoteAddr
	return performRateLimitRawRequest(e, req)
}

func performRateLimitRawRequest(e *fox.Engine, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

type fakeRedisRateLimitClient struct {
	result       any
	err          error
	keys         []string
	args         []any
	evalShaCalls int
	evalCalls    int
}

func (f *fakeRedisRateLimitClient) Eval(ctx context.Context, _ string, keys []string, args ...any) *goredis.Cmd {
	f.evalCalls++
	f.keys = append([]string(nil), keys...)
	f.args = append([]any(nil), args...)
	return goredis.NewCmdResult(f.result, f.err)
}

func (f *fakeRedisRateLimitClient) EvalSha(ctx context.Context, _ string, keys []string, args ...any) *goredis.Cmd {
	f.evalShaCalls++
	f.keys = append([]string(nil), keys...)
	f.args = append([]any(nil), args...)
	return goredis.NewCmdResult(f.result, f.err)
}

func (f *fakeRedisRateLimitClient) EvalRO(ctx context.Context, script string, keys []string, args ...any) *goredis.Cmd {
	return f.Eval(ctx, script, keys, args...)
}

func (f *fakeRedisRateLimitClient) EvalShaRO(ctx context.Context, sha1 string, keys []string, args ...any) *goredis.Cmd {
	return f.EvalSha(ctx, sha1, keys, args...)
}

func (f *fakeRedisRateLimitClient) ScriptExists(ctx context.Context, hashes ...string) *goredis.BoolSliceCmd {
	return goredis.NewBoolSliceResult([]bool{true}, nil)
}

func (f *fakeRedisRateLimitClient) ScriptLoad(ctx context.Context, script string) *goredis.StringCmd {
	return goredis.NewStringResult("sha", nil)
}
