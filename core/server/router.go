package server

import "net/http"

type RouterGroup struct {
	prefix   string
	engine   *Engine
	handlers HandlersChain
}

// Group 创建子路由分组
func (rg *RouterGroup) Group(prefix string, middlewares ...HandlerFunc) *RouterGroup {
	handlers := cloneHandlers(rg.handlers)
	handlers = append(handlers, middlewares...)
	return &RouterGroup{
		prefix:   rg.prefix + prefix,
		engine:   rg.engine,
		handlers: handlers,
	}
}

// Use 为当前分组添加中间件
func (rg *RouterGroup) Use(middlewares ...HandlerFunc) {
	rg.handlers = append(rg.handlers, middlewares...)
}

// GET 注册 GET 路由
func (rg *RouterGroup) GET(path string, handler HandlerFunc) {
	rg.handle("GET", path, handler)
}

// POST 注册 POST 路由
func (rg *RouterGroup) POST(path string, handler HandlerFunc) {
	rg.handle("POST", path, handler)
}

// PUT 注册 PUT 路由
func (rg *RouterGroup) PUT(path string, handler HandlerFunc) {
	rg.handle("PUT", path, handler)
}

// DELETE 注册 DELETE 路由
func (rg *RouterGroup) DELETE(path string, handler HandlerFunc) {
	rg.handle("DELETE", path, handler)
}

// PATCH 注册 PATCH 路由
func (rg *RouterGroup) PATCH(path string, handler HandlerFunc) {
	rg.handle("PATCH", path, handler)
}

// HEAD 注册 HEAD 路由
func (rg *RouterGroup) HEAD(path string, handler HandlerFunc) {
	rg.handle("HEAD", path, handler)
}

// OPTIONS 注册 OPTIONS 路由
func (rg *RouterGroup) OPTIONS(path string, handler HandlerFunc) {
	rg.handle("OPTIONS", path, handler)
}

// Any 注册所有 HTTP 方法的路由
func (rg *RouterGroup) Any(path string, handler HandlerFunc) {
	for _, method := range anyMethods {
		rg.handle(method, path, handler)
	}
}

// Static 注册静态文件服务，将 URL 路径映射到本地文件系统目录
func (rg *RouterGroup) Static(relativePath, root string) {
	rg.engine.engine.Static(rg.prefix+relativePath, root)
}

// StaticFile 注册单个静态文件路由
func (rg *RouterGroup) StaticFile(relativePath, filepath string) {
	rg.engine.engine.StaticFile(rg.prefix+relativePath, filepath)
}

// StaticFS 注册静态文件服务，使用自定义的 http.FileSystem
func (rg *RouterGroup) StaticFS(relativePath string, fs http.FileSystem) {
	rg.engine.engine.StaticFS(rg.prefix+relativePath, fs)
}

// handle 统一处理路由注册
func (rg *RouterGroup) handle(method, path string, handler HandlerFunc) {
	fullPath := rg.prefix + path
	handlers := make(HandlersChain, len(rg.handlers)+1)
	copy(handlers, rg.handlers)
	handlers[len(rg.handlers)] = handler
	rg.engine.registerHandlers(method, fullPath, handlers)
}

func cloneHandlers(handlers HandlersChain) HandlersChain {
	if len(handlers) == 0 {
		return nil
	}
	cloned := make(HandlersChain, len(handlers))
	copy(cloned, handlers)
	return cloned
}
