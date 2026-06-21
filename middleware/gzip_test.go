package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/surge-go/fox"
)

func newGzipTestEngine(handler fox.HandlerFunc) *fox.Engine {
	e := fox.New(&fox.Config{Addr: ":0", Mode: fox.ModeTest, PrintRoutes: boolPtr(false)})
	e.Use(handler)
	e.GET("/json", func(c *fox.Context) {
		c.JSON(http.StatusOK, map[string]string{"message": strings.Repeat("hello", 300)})
	})
	e.GET("/small", func(c *fox.Context) {
		c.String(http.StatusOK, "ok")
	})
	e.GET("/image.png", func(c *fox.Context) {
		c.SetHeader("Content-Type", "image/png")
		c.String(http.StatusOK, strings.Repeat("png", 300))
	})
	e.GET("/encoded", func(c *fox.Context) {
		c.SetHeader("Content-Encoding", "br")
		c.String(http.StatusOK, strings.Repeat("encoded", 300))
	})
	e.GET("/stream", func(c *fox.Context) {
		c.SetHeader("Content-Type", "text/event-stream")
		c.String(http.StatusOK, strings.Repeat("event", 300))
	})
	e.GET("/empty", func(c *fox.Context) {
		c.AbortWithStatus(http.StatusNoContent)
	})
	e.GET("/empty-write", func(c *fox.Context) {
		c.RawWriter().WriteHeader(http.StatusNoContent)
		_, _ = c.RawWriter().Write([]byte("should-not-write"))
	})
	e.GET("/empty-flush-write", func(c *fox.Context) {
		c.RawWriter().WriteHeader(http.StatusNoContent)
		c.RawWriter().(http.Flusher).Flush()
		_, _ = c.RawWriter().Write([]byte("should-not-write"))
	})
	e.GET("/flush", func(c *fox.Context) {
		c.SetHeader("Content-Type", "text/plain; charset=utf-8")
		c.RawWriter().(http.Flusher).Flush()
		c.String(http.StatusOK, strings.Repeat("flush", 300))
	})
	e.GET("/problem", func(c *fox.Context) {
		c.SetHeader("Content-Type", "application/problem+json")
		_, _ = c.RawWriter().Write([]byte(strings.Repeat(`{"message":"problem"}`, 100)))
	})
	e.GET("/panic", func(c *fox.Context) {
		panic("boom")
	})
	e.GET("/panic-after-write", func(c *fox.Context) {
		c.String(http.StatusOK, "partial")
		panic("boom")
	})
	e.GET("/written", func(c *fox.Context) {
		c.String(http.StatusOK, "ok")
	}, func(c *fox.Context) {
		if c.Written() {
			c.SetHeader("X-Written", "true")
			return
		}
		c.SetHeader("X-Written", "false")
		c.String(http.StatusAccepted, "second")
	})
	return e
}

func TestGzipCompressesLargeTextResponse(t *testing.T) {
	e := newGzipTestEngine(Gzip())

	rec := performGzipRequest(e, http.MethodGet, "/json", "gzip")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Encoding"); got != gzipEncoding {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary = %q, want Accept-Encoding", got)
	}
	body := gunzipBody(t, rec.Body.Bytes())
	if !strings.Contains(body, "hellohello") {
		t.Fatalf("body = %q, want decompressed json", body)
	}
}

func TestGzipSkipsWhenClientDoesNotAcceptGzip(t *testing.T) {
	e := newGzipTestEngine(Gzip())

	rec := performGzipRequest(e, http.MethodGet, "/json", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "hellohello") {
		t.Fatalf("body = %q, want plain json", body)
	}
}

func TestGzipSkipsWhenGzipQualityIsZero(t *testing.T) {
	e := newGzipTestEngine(Gzip())

	rec := performGzipRequest(e, http.MethodGet, "/json", "br, gzip;q=0")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
}

func TestGzipAcceptsWildcardEncoding(t *testing.T) {
	e := newGzipTestEngine(Gzip())

	rec := performGzipRequest(e, http.MethodGet, "/json", "br, *;q=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Encoding"); got != gzipEncoding {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
}

func TestGzipSkipsSmallResponse(t *testing.T) {
	e := newGzipTestEngine(Gzip())

	rec := performGzipRequest(e, http.MethodGet, "/small", "gzip")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if body := rec.Body.String(); body != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
}

func TestGzipWrittenIsVisibleBeforeBufferedResponseFlushes(t *testing.T) {
	e := newGzipTestEngine(Gzip())

	rec := performGzipRequest(e, http.MethodGet, "/written", "gzip")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("X-Written"); got != "true" {
		t.Fatalf("X-Written = %q, want true", got)
	}
	if body := rec.Body.String(); body != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
}

func TestGzipSkipsExcludedExtension(t *testing.T) {
	e := newGzipTestEngine(Gzip())

	rec := performGzipRequest(e, http.MethodGet, "/image.png", "gzip")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
}

func TestGzipCompressesWhenFlushHappensBeforeBody(t *testing.T) {
	e := newGzipTestEngine(GzipWithConfig(GzipConfig{MinSize: 1}))

	rec := performGzipRequest(e, http.MethodGet, "/flush", "gzip")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Encoding"); got != gzipEncoding {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	body := gunzipBody(t, rec.Body.Bytes())
	if !strings.Contains(body, "flushflush") {
		t.Fatalf("body = %q, want decompressed flush response", body)
	}
}

func TestGzipSkipsAlreadyEncodedResponse(t *testing.T) {
	e := newGzipTestEngine(Gzip())

	rec := performGzipRequest(e, http.MethodGet, "/encoded", "gzip")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want br", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "encodedencoded") {
		t.Fatalf("body = %q, want plain encoded response", body)
	}
}

func TestGzipCustomContentTypesAreStrict(t *testing.T) {
	e := newGzipTestEngine(GzipWithConfig(GzipConfig{
		MinSize:      1,
		ContentTypes: []string{"text/plain"},
	}))

	rec := performGzipRequest(e, http.MethodGet, "/problem", "gzip")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "problem") {
		t.Fatalf("body = %q, want plain problem response", body)
	}
}

func TestGzipSkipsEventStream(t *testing.T) {
	e := newGzipTestEngine(GzipWithConfig(GzipConfig{MinSize: 1}))

	rec := performGzipRequest(e, http.MethodGet, "/stream", "gzip")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
}

func TestGzipSkipsNoBodyResponses(t *testing.T) {
	e := newGzipTestEngine(GzipWithConfig(GzipConfig{MinSize: 1}))

	rec := performGzipRequest(e, http.MethodGet, "/empty", "gzip")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
}

func TestGzipDropsBufferedBodyForNoBodyStatus(t *testing.T) {
	e := newGzipTestEngine(GzipWithConfig(GzipConfig{MinSize: 1}))

	rec := performGzipRequest(e, http.MethodGet, "/empty-write", "gzip")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if body := rec.Body.String(); body != "" {
		t.Fatalf("body = %q, want empty", body)
	}

	rec = performGzipRequest(e, http.MethodGet, "/empty-flush-write", "gzip")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("flush status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if body := rec.Body.String(); body != "" {
		t.Fatalf("flush body = %q, want empty", body)
	}
}

func TestGzipDoesNotWriteBufferedResponseOnPanic(t *testing.T) {
	e := newGzipTestEngine(Gzip())

	rec := performGzipRequest(e, http.MethodGet, "/panic", "gzip")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}

	rec = performGzipRequest(e, http.MethodGet, "/panic-after-write", "gzip")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
}

func TestGzipSkipPathAndSkipFunc(t *testing.T) {
	t.Run("path", func(t *testing.T) {
		e := newGzipTestEngine(GzipWithConfig(GzipConfig{SkipPaths: []string{"/json"}}))

		rec := performGzipRequest(e, http.MethodGet, "/json", "gzip")
		if got := rec.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("Content-Encoding = %q, want empty", got)
		}
	})

	t.Run("func", func(t *testing.T) {
		e := newGzipTestEngine(GzipWithConfig(GzipConfig{
			SkipFunc: func(c *fox.Context) bool {
				return c.RawRequest().URL.Path == "/json"
			},
		}))

		rec := performGzipRequest(e, http.MethodGet, "/json", "gzip")
		if got := rec.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("Content-Encoding = %q, want empty", got)
		}
	})
}

func performGzipRequest(e *fox.Engine, method, path, acceptEncoding string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func gunzipBody(t *testing.T, body []byte) string {
	t.Helper()
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read gzip body: %v", err)
	}
	return string(data)
}
