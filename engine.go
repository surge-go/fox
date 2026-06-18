package fox

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
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

var ginModeOnce sync.Once

// Engine 是 fox 的 HTTP server 核心。
//
// 它负责持有底层 gin.Engine 和标准库 http.Server，并统一管理路由注册、
// 启动、优雅关闭以及启动时路由表打印所需的 RouteInfo 快照。
type Engine struct {
	// mode 是当前 server 运行模式，来自 Config.Mode 的规范化结果。
	mode Mode
	// engine 是底层 Gin 引擎，负责真实的路由匹配和请求分发。
	engine *gin.Engine
	// server 是标准库 HTTP server，负责监听、TLS、超时和优雅关闭。
	server *http.Server
	// cfg 是经过默认值填充和地址规范化后的配置副本。
	cfg *Config
	// errors 是当前 Engine 使用的错误工厂，不依赖包级全局状态。
	errors IErrors

	// mu 保护 routes 和 sealed。路由通常在启动前注册，但这里仍保证快照读取安全。
	mu     sync.RWMutex
	sealed bool
	routes []RouteInfo
}

// New 创建 HTTP server 引擎。
//
// cfg 为 nil 时会使用默认配置；配置不合法时会 panic。
// err 可传入业务自定义的错误工厂，不传时使用默认 Err。
func New(cfg *Config, err ...IErrors) *Engine {
	errorFactory := newEngineErrorFactory(err...)
	cfgCopy := defaultConfig(cfg)
	if validateErr := cfgCopy.Validate(); validateErr != nil {
		panic(fmt.Errorf("fox: invalid config: %w", validateErr))
	}

	cfgCopy.Mode = cfgCopy.modeOrDefault()
	cfgCopy.Addr = normalizeAddr(cfgCopy.Addr)

	// gin.SetMode 修改的是进程级全局状态；fox 固定让底层 Gin 使用 release
	// 模式，避免输出 [GIN-debug] 路由和模式警告。fox 自身的运行模式以
	// Engine.mode 为准，仍可控制 recovery 和路由打印等框架行为。
	ginModeOnce.Do(func() {
		gin.SetMode(gin.ReleaseMode)
	})

	engine := gin.New()
	if cfgCopy.MaxMultipartMemory > 0 {
		engine.MaxMultipartMemory = int64(cfgCopy.MaxMultipartMemory)
	}
	if setProxyErr := engine.SetTrustedProxies(cfgCopy.TrustedProxies); setProxyErr != nil {
		panic(fmt.Errorf("fox: set trusted proxies: %w", setProxyErr))
	}

	server := &http.Server{
		Addr:           cfgCopy.Addr,
		Handler:        engine,
		ReadTimeout:    cfgCopy.ReadTimeout,
		WriteTimeout:   cfgCopy.WriteTimeout,
		MaxHeaderBytes: cfgCopy.MaxHeaderBytes,
	}
	// TLSConfig.Config 优先级最高；未提供时使用证书文件和简化 TLS 选项。
	if cfgCopy.TLS != nil {
		if cfgCopy.TLS.Config != nil {
			server.TLSConfig = cfgCopy.TLS.Config
		} else {
			server.TLSConfig = &tls.Config{
				MinVersion:   cfgCopy.TLS.MinVersion,
				CipherSuites: cfgCopy.TLS.CipherSuites,
			}
		}
	}
	// h2c 只在未启用 TLS 时通过配置校验。
	if cfgCopy.UseH2C {
		server.Handler = h2c.NewHandler(engine, &http2.Server{})
	}

	e := &Engine{
		mode:   cfgCopy.Mode,
		engine: engine,
		server: server,
		cfg:    &cfgCopy,
		errors: errorFactory,
	}
	e.Use(recoveryMiddleware(e.mode))
	return e
}

// Run 启动 HTTP server，阻塞直到收到 SIGINT/SIGTERM 或发生错误。
//
// 收到系统信号时会统一调用 Shutdown，并最多等待 Config.ShutdownTimeout。
func (e *Engine) Run() error {
	if err := e.ready(); err != nil {
		return err
	}
	e.sealRoutes()
	if e.cfg.printRoutesEnabled() {
		e.printRoutes()
	}

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
		if err := e.shutdownWithTimeout(e.cfg.ShutdownTimeout); err != nil {
			return err
		}
		return <-errChan
	}
}

// Shutdown 优雅关闭 HTTP server。
//
// ctx 为 nil 时会使用 context.Background()。该方法是主动关闭和 Run 信号关闭
// 共同使用的统一入口。
func (e *Engine) Shutdown(ctx context.Context) error {
	if err := e.ready(); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return e.server.Shutdown(ctx)
}

// shutdownWithTimeout 使用固定超时时间包装 Shutdown。
//
// 该方法主要供 Run 在收到系统信号时调用，保证信号关闭和主动关闭复用同一逻辑。
func (e *Engine) shutdownWithTimeout(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return e.Shutdown(ctx)
}

// ServeHTTP 实现 http.Handler，便于测试或与标准库组合使用。
//
// 实现该方法后，*Engine 可以直接传给 httptest、http.Server 或其它接受
// http.Handler 的组件。
func (e *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if err := e.ready(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	e.engine.ServeHTTP(w, req)
}

// Use 注册全局中间件。
//
// 全局中间件会作用于后续注册的所有路由。
func (e *Engine) Use(middlewares ...HandlerFunc) {
	mustHaveHandlers(middlewares)
	if e == nil || e.engine == nil {
		panic("fox: nil engine")
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	e.mustAllowRouteRegistrationLocked()
	e.engine.Use(toGinHandlers(middlewares, e.errors)...)
}

// UseGin 注册原生 Gin 中间件。
//
// 该方法用于复用 Gin 生态中间件；普通业务代码优先使用 Use。
func (e *Engine) UseGin(middlewares ...gin.HandlerFunc) {
	mustHaveGinHandlers(middlewares)
	if e == nil || e.engine == nil {
		panic("fox: nil engine")
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	e.mustAllowRouteRegistrationLocked()
	e.engine.Use(middlewares...)
}

// Group 创建路由分组。
//
// 分组会保存路径前缀和中间件，最终仍通过 Engine 的统一 handle 方法注册。
func (e *Engine) Group(prefix string, middlewares ...HandlerFunc) *RouteGroup {
	mustNotContainNilHandlers(middlewares)
	return &RouteGroup{
		prefix:   joinPaths("", prefix),
		engine:   e,
		handlers: cloneHandlers(middlewares),
	}
}

// GET 注册 GET 路由。
func (e *Engine) GET(path string, handlers ...HandlerFunc) {
	e.handle(http.MethodGet, path, handlers...)
}

// POST 注册 POST 路由。
func (e *Engine) POST(path string, handlers ...HandlerFunc) {
	e.handle(http.MethodPost, path, handlers...)
}

// PUT 注册 PUT 路由。
func (e *Engine) PUT(path string, handlers ...HandlerFunc) {
	e.handle(http.MethodPut, path, handlers...)
}

// DELETE 注册 DELETE 路由。
func (e *Engine) DELETE(path string, handlers ...HandlerFunc) {
	e.handle(http.MethodDelete, path, handlers...)
}

// HEAD 注册 HEAD 路由。
func (e *Engine) HEAD(path string, handlers ...HandlerFunc) {
	e.handle(http.MethodHead, path, handlers...)
}

// OPTIONS 注册 OPTIONS 路由。
func (e *Engine) OPTIONS(path string, handlers ...HandlerFunc) {
	e.handle(http.MethodOptions, path, handlers...)
}

// PATCH 注册 PATCH 路由。
func (e *Engine) PATCH(path string, handlers ...HandlerFunc) {
	e.handle(http.MethodPatch, path, handlers...)
}

// Any 为同一路径注册常见 HTTP 方法。
func (e *Engine) Any(path string, handlers ...HandlerFunc) {
	for _, method := range anyMethods {
		e.handle(method, path, handlers...)
	}
}

// Static 注册静态目录。
//
// 除了注册到底层 Gin，也会记录 GET 和 HEAD 路由，供启动路由表打印。
func (e *Engine) Static(relativePath, root string) {
	if err := e.ready(); err != nil {
		panic(err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.mustAllowRouteRegistrationLocked()
	e.engine.Static(relativePath, root)
	e.appendRoute(http.MethodGet, relativePath+"/*filepath", "static:"+root)
	e.appendRoute(http.MethodHead, relativePath+"/*filepath", "static:"+root)
}

// StaticFile 注册单个静态文件。
//
// 除了注册到底层 Gin，也会记录 GET 和 HEAD 路由，供启动路由表打印。
func (e *Engine) StaticFile(relativePath, filepath string) {
	if err := e.ready(); err != nil {
		panic(err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.mustAllowRouteRegistrationLocked()
	e.engine.StaticFile(relativePath, filepath)
	e.appendRoute(http.MethodGet, relativePath, "staticFile:"+filepath)
	e.appendRoute(http.MethodHead, relativePath, "staticFile:"+filepath)
}

// StaticFS 注册静态文件服务。
//
// 除了注册到底层 Gin，也会记录 GET 和 HEAD 路由，供启动路由表打印。
func (e *Engine) StaticFS(relativePath string, fsys http.FileSystem) {
	if err := e.ready(); err != nil {
		panic(err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.mustAllowRouteRegistrationLocked()
	e.engine.StaticFS(relativePath, fsys)
	e.appendRoute(http.MethodGet, relativePath+"/*filepath", "staticFS")
	e.appendRoute(http.MethodHead, relativePath+"/*filepath", "staticFS")
}

// StaticFSFromFS 注册标准库 fs.FS 静态文件服务。
func (e *Engine) StaticFSFromFS(relativePath string, fsys fs.FS) {
	e.StaticFS(relativePath, http.FS(fsys))
}

// handle 是所有 HTTP 方法注册的统一入口。
//
// 它会校验处理链、注册到底层 Gin，并同步记录 RouteInfo。
func (e *Engine) handle(method, path string, handlers ...HandlerFunc) {
	if e == nil || e.engine == nil {
		panic("fox: nil engine")
	}
	mustHaveHandlers(handlers)

	e.mu.Lock()
	defer e.mu.Unlock()
	e.mustAllowRouteRegistrationLocked()

	e.engine.Handle(method, path, toGinHandlers(handlers, e.errors)...)
	e.appendRoute(method, path, displayRouteHandlerName(handlers[len(handlers)-1]))
}

// recordRoute 记录不经过 handle 的路由，例如静态文件路由。
func (e *Engine) recordRoute(method, path, handler string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.appendRoute(method, path, handler)
}

// appendRoute 在调用方已持有 e.mu 时追加路由快照。
func (e *Engine) appendRoute(method, path, handler string) {
	e.routes = append(e.routes, RouteInfo{
		Method:  method,
		Path:    path,
		Handler: handler,
	})
}

// sealRoutes 标记路由表已进入运行期，不再允许修改底层 gin 路由树。
func (e *Engine) sealRoutes() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.sealed = true
}

func (e *Engine) mustAllowRouteRegistrationLocked() {
	if e.sealed {
		panic("fox: routes are sealed")
	}
}

// ready 检查 Engine 是否已经完成初始化。
//
// 对外生命周期方法使用 error 返回，避免未初始化 Engine 导致 nil panic。
func (e *Engine) ready() error {
	if e == nil {
		return errors.New("fox: nil engine")
	}
	if e.engine == nil {
		return errors.New("fox: nil gin engine")
	}
	if e.server == nil {
		return errors.New("fox: nil http server")
	}
	if e.cfg == nil {
		return errors.New("fox: nil config")
	}
	return nil
}

// mustHaveHandlers 校验 fox HandlerFunc 处理链。
//
// 路由和中间件都至少需要一个非 nil 处理函数。
func mustHaveHandlers(handlers HandlersChain) {
	if len(handlers) == 0 {
		panic("fox: route must have at least one handler")
	}
	mustNotContainNilHandlers(handlers)
}

// mustNotContainNilHandlers 校验可选中间件列表，允许为空但不允许 nil。
func mustNotContainNilHandlers(handlers HandlersChain) {
	for _, handler := range handlers {
		if handler == nil {
			panic("fox: nil handler")
		}
	}
}

// mustHaveGinHandlers 校验原生 Gin 中间件处理链。
func mustHaveGinHandlers(handlers []gin.HandlerFunc) {
	if len(handlers) == 0 {
		panic("fox: route must have at least one handler")
	}
	for _, handler := range handlers {
		if handler == nil {
			panic("fox: nil gin handler")
		}
	}
}
