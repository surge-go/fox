package fox

import (
	"net/http"
	"strings"
)

// anyMethods 定义 Any 路由需要注册的 HTTP 方法集合。
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

// RouteGroup 表示一组共享路径前缀和中间件的路由。
//
// prefix 会与子分组或路由 path 直接拼接；handlers 会在注册具体路由时
// 放在业务 handlers 前面，形成最终的处理链。
type RouteGroup struct {
	prefix   string
	engine   *Engine
	handlers HandlersChain
}

// Use 为当前路由组追加中间件。
//
// 追加后的中间件只影响后续通过当前 RouteGroup 注册的路由。
func (r *RouteGroup) Use(middlewares ...HandlerFunc) {
	mustNotContainNilHandlers(middlewares)
	r.handlers = append(r.handlers, middlewares...)
}

// Group 创建子路由组。
//
// 子路由组会继承当前组的 prefix 和 handlers，并追加自己的 prefix 与中间件。
func (r *RouteGroup) Group(prefix string, middlewares ...HandlerFunc) *RouteGroup {
	mustNotContainNilHandlers(middlewares)
	handlers := cloneHandlers(r.handlers)
	handlers = append(handlers, middlewares...)
	return &RouteGroup{
		prefix:   joinPaths(r.prefix, prefix),
		engine:   r.engine,
		handlers: handlers,
	}
}

// GET 注册 GET 路由。
func (r *RouteGroup) GET(path string, handlers ...HandlerFunc) {
	r.handle(http.MethodGet, path, handlers...)
}

// POST 注册 POST 路由。
func (r *RouteGroup) POST(path string, handlers ...HandlerFunc) {
	r.handle(http.MethodPost, path, handlers...)
}

// PUT 注册 PUT 路由。
func (r *RouteGroup) PUT(path string, handlers ...HandlerFunc) {
	r.handle(http.MethodPut, path, handlers...)
}

// DELETE 注册 DELETE 路由。
func (r *RouteGroup) DELETE(path string, handlers ...HandlerFunc) {
	r.handle(http.MethodDelete, path, handlers...)
}

// HEAD 注册 HEAD 路由。
func (r *RouteGroup) HEAD(path string, handlers ...HandlerFunc) {
	r.handle(http.MethodHead, path, handlers...)
}

// OPTIONS 注册 OPTIONS 路由。
func (r *RouteGroup) OPTIONS(path string, handlers ...HandlerFunc) {
	r.handle(http.MethodOptions, path, handlers...)
}

// PATCH 注册 PATCH 路由。
func (r *RouteGroup) PATCH(path string, handlers ...HandlerFunc) {
	r.handle(http.MethodPatch, path, handlers...)
}

// Any 为同一路径注册常见 HTTP 方法。
func (r *RouteGroup) Any(path string, handlers ...HandlerFunc) {
	for _, method := range anyMethods {
		r.handle(method, path, handlers...)
	}
}

// handle 合并分组中间件和当前路由处理函数，并注册到底层 gin engine。
func (r *RouteGroup) handle(method, path string, handlers ...HandlerFunc) {
	if r == nil || r.engine == nil {
		panic("fox: nil route group engine")
	}

	chain := cloneHandlers(r.handlers)
	chain = append(chain, handlers...)
	r.engine.handle(method, joinPaths(r.prefix, path), chain...)
}

func joinPaths(prefix, path string) string {
	prefix = strings.TrimSpace(prefix)
	path = strings.TrimSpace(path)
	if prefix == "" {
		if path == "" {
			return "/"
		}
		if strings.HasPrefix(path, "/") {
			return path
		}
		return "/" + path
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if path == "" || path == "/" {
		return prefix
	}
	return strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(path, "/")
}

// cloneHandlers 复制处理链，避免子分组追加中间件时修改父分组切片。
func cloneHandlers(handlers HandlersChain) HandlersChain {
	if len(handlers) == 0 {
		return nil
	}
	cloned := make(HandlersChain, len(handlers))
	copy(cloned, handlers)
	return cloned
}
