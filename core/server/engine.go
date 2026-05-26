package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

var anyMethods = []string{
	http.MethodGet,
	http.MethodPost,
	http.MethodPut,
	http.MethodDelete,
	http.MethodPatch,
	http.MethodHead,
	http.MethodOptions,
	http.MethodConnect,
	http.MethodTrace,
}

type Engine struct {
	mode             Mode
	engine           *gin.Engine
	server           *http.Server
	cfg              *Config
	mu               sync.Mutex
	routes           []RouteInfo
	docRoutes        map[*Route]struct{}
	anonFuncCounters map[string]int
}

// New 创建 HTTP 服务器引擎
func New(cfg *Config) (*Engine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	cfgCopy := *cfg
	cfgCopy.Mode = cfg.modeOrDefault()
	cfgCopy.Addr = normalizeAddr(cfg.Addr)

	gin.SetMode(string(cfgCopy.Mode))

	// 禁用 Gin 默认日志
	gin.DefaultWriter = io.Discard
	gin.DefaultErrorWriter = io.Discard

	engine := gin.New()

	if cfgCopy.MaxMultipartMemory > 0 {
		engine.MaxMultipartMemory = int64(cfgCopy.MaxMultipartMemory)
	}

	if err := engine.SetTrustedProxies(cfgCopy.TrustedProxies); err != nil {
		return nil, fmt.Errorf("set trusted proxies: %w", err)
	}

	srv := &http.Server{
		Addr:           cfgCopy.Addr,
		Handler:        engine,
		ReadTimeout:    cfgCopy.ReadTimeout,
		WriteTimeout:   cfgCopy.WriteTimeout,
		MaxHeaderBytes: cfgCopy.MaxHeaderBytes,
	}

	if cfgCopy.TLS != nil {
		if cfgCopy.TLS.Config != nil {
			srv.TLSConfig = cfgCopy.TLS.Config
		} else {
			srv.TLSConfig = &tls.Config{
				MinVersion:   cfgCopy.TLS.MinVersion,
				CipherSuites: cfgCopy.TLS.CipherSuites,
			}
		}
	}

	if cfgCopy.UseH2C {
		srv.Handler = h2c.NewHandler(engine, &http2.Server{})
	}

	e := &Engine{
		mode:             cfgCopy.Mode,
		engine:           engine,
		server:           srv,
		cfg:              &cfgCopy,
		docRoutes:        make(map[*Route]struct{}),
		anonFuncCounters: make(map[string]int),
	}

	// 自动注册内置中间件
	e.Use(recoveryMiddleware(e.mode))
	if cfgCopy.loggerEnabled() {
		e.Use(loggerMiddleware())
	}

	return e, nil
}

// Run 启动 HTTP 服务器，阻塞直到收到系统信号（SIGINT/SIGTERM）或发生错误。
// 收到信号后自动执行优雅关闭，最多等待 30 秒。
func (e *Engine) Run() error {
	e.printRoutes()

	errChan := make(chan error, 1)

	go func() {
		var err error
		if e.cfg.TLS != nil {
			if e.cfg.TLS.Config != nil {
				err = e.server.ListenAndServeTLS("", "")
			} else {
				err = e.server.ListenAndServeTLS(e.cfg.TLS.CertFile, e.cfg.TLS.KeyFile)
			}
		} else {
			err = e.server.ListenAndServe()
		}
		if errors.Is(err, http.ErrServerClosed) {
			errChan <- nil
			return
		}
		errChan <- err
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	select {
	case err := <-errChan:
		return err
	case <-quit:
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := e.server.Shutdown(ctx); err != nil {
			return err
		}
		return <-errChan
	}
}

// Shutdown 优雅关闭服务器
func (e *Engine) Shutdown(ctx context.Context) error {
	return e.server.Shutdown(ctx)
}

// Use 注册全局中间件
func (e *Engine) Use(middlewares ...HandlerFunc) {
	e.engine.Use(toGinHandlers(middlewares)...)
}

// Group 创建路由分组
func (e *Engine) Group(prefix string, middlewares ...HandlerFunc) *RouterGroup {
	return &RouterGroup{
		prefix:   prefix,
		engine:   e,
		handlers: cloneHandlers(middlewares),
	}
}

// GET 注册 GET 路由
func (e *Engine) GET(path string, handler HandlerFunc) *Route {
	return e.registerRoute(http.MethodGet, path, handler)
}

// POST 注册 POST 路由
func (e *Engine) POST(path string, handler HandlerFunc) *Route {
	return e.registerRoute(http.MethodPost, path, handler)
}

// PUT 注册 PUT 路由
func (e *Engine) PUT(path string, handler HandlerFunc) *Route {
	return e.registerRoute(http.MethodPut, path, handler)
}

// DELETE 注册 DELETE 路由
func (e *Engine) DELETE(path string, handler HandlerFunc) *Route {
	return e.registerRoute(http.MethodDelete, path, handler)
}

// PATCH 注册 PATCH 路由
func (e *Engine) PATCH(path string, handler HandlerFunc) *Route {
	return e.registerRoute(http.MethodPatch, path, handler)
}

// HEAD 注册 HEAD 路由
func (e *Engine) HEAD(path string, handler HandlerFunc) *Route {
	return e.registerRoute(http.MethodHead, path, handler)
}

// OPTIONS 注册 OPTIONS 路由
func (e *Engine) OPTIONS(path string, handler HandlerFunc) *Route {
	return e.registerRoute(http.MethodOptions, path, handler)
}

// Any 注册所有 HTTP 方法的路由
func (e *Engine) Any(path string, handler HandlerFunc) []*Route {
	return e.registerAny(path, handler)
}

// Static 注册静态文件服务，将 URL 路径映射到本地文件系统目录
func (e *Engine) Static(relativePath, root string) {
	e.engine.Static(relativePath, root)
}

// StaticFile 注册单个静态文件路由
func (e *Engine) StaticFile(relativePath, filepath string) {
	e.engine.StaticFile(relativePath, filepath)
}

// StaticFS 注册静态文件服务，使用自定义的 http.FileSystem
func (e *Engine) StaticFS(relativePath string, fs http.FileSystem) {
	e.engine.StaticFS(relativePath, fs)
}

// ServeHTTP 实现 http.Handler 接口，用于测试
func (e *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	e.engine.ServeHTTP(w, req)
}

func (e *Engine) registerRoute(method, path string, handler HandlerFunc) *Route {
	e.registerHandlers(method, path, HandlersChain{handler})
	route := newRoute(e, method, path, handler, nil)
	route.markForDocIfDocumentable()
	return route
}

func (e *Engine) registerAny(path string, handler HandlerFunc) []*Route {
	routes := make([]*Route, 0, len(anyMethods))
	func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		for _, method := range anyMethods {
			e.registerHandlersLocked(method, path, HandlersChain{handler})
			route := newRoute(e, method, path, handler, nil)
			routes = append(routes, route)
		}
	}()

	for _, route := range routes {
		route.markForDocIfDocumentable()
	}
	return routes
}

func (e *Engine) registerHandlers(method, path string, handlers HandlersChain) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.registerHandlersLocked(method, path, handlers)
}

func (e *Engine) registerHandlersLocked(method, path string, handlers HandlersChain) {
	e.engine.Handle(method, path, toGinHandlers(handlers)...)
	if len(handlers) == 0 {
		return
	}
	e.routes = append(e.routes, RouteInfo{
		Method:  method,
		Path:    path,
		Handler: e.getHandlerName(handlers[len(handlers)-1]),
	})
}

func (e *Engine) routeSnapshot() []RouteInfo {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.routes) == 0 {
		return nil
	}

	snapshot := make([]RouteInfo, len(e.routes))
	copy(snapshot, e.routes)
	return snapshot
}
