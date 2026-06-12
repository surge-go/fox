package server

import (
	"fmt"
	"net"
	"net/http"
	"reflect"
	"runtime"
	"strings"
)

// RouteInfo 路由信息
type RouteInfo struct {
	Method  string
	Path    string
	Handler string
	Hidden  bool
}

// methodColor 返回 HTTP 方法对应的 ANSI 颜色代码
func methodColor(method string) string {
	switch method {
	case http.MethodGet:
		return colorBlue
	case http.MethodPost:
		return colorGreen
	case http.MethodPut:
		return colorYellow
	case http.MethodDelete:
		return colorRed
	case http.MethodPatch:
		return colorCyan
	case http.MethodHead:
		return colorMagenta
	case http.MethodOptions:
		return colorWhite
	default:
		return colorReset
	}
}

const colorReset = "\033[0m"
const colorRed = "\033[31m"     // 红色
const colorBlue = "\033[34m"    // 蓝色
const colorCyan = "\033[36m"    // 青色
const colorYellow = "\033[33m"  // 黄色
const colorGreen = "\033[32m"   // 绿色
const colorMagenta = "\033[35m" // 紫色
const colorWhite = "\033[37m"   // 白色

// printRoutes 打印路由表
func (e *Engine) printRoutes() {
	routes := e.routeSnapshot()
	if len(routes) == 0 {
		return
	}

	e.printBanner()
	for _, r := range routes {
		if r.Hidden {
			continue
		}
		color := methodColor(r.Method)
		fmt.Printf("%s[Fox-debug]%s %s%-7s%s %-30s --> %s\n",
			colorCyan, colorReset, color, r.Method, colorReset, r.Path, r.Handler)
	}
	fmt.Println()
	e.printOpenAPIInfo()
}

func (e *Engine) printBanner() {
	runtimeText := runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH
	fmt.Printf(`
%s        ______
%s       / ____/___  _  __
%s      / /_  / __ \| |/_/
%s     / __/ / /_/ />  <
%s    /_/    \____/_/|_|

%s    Fox Web 框架%s
%s    快速构建 · 可观测 · 面向生产的 Go 服务%s

%s    ------------------------------------------------------------
%s    运行模式  %s%s%s
%s    监听地址  %s%s%s
%s    运行环境  %s%s%s
%s    ------------------------------------------------------------
%s
`,
		colorCyan,
		colorCyan,
		colorCyan,
		colorCyan,
		colorCyan,
		colorGreen,
		colorReset,
		colorWhite,
		colorReset,
		colorCyan,
		colorWhite,
		colorMagenta,
		e.mode,
		colorReset,
		colorWhite,
		colorGreen,
		e.publicBaseURL(),
		colorReset,
		colorWhite,
		colorGreen,
		runtimeText,
		colorReset,
		colorCyan,
		colorReset,
	)
}

func (e *Engine) printOpenAPIInfo() {
	if e == nil || e.cfg == nil || e.cfg.OpenAPI == nil {
		return
	}

	baseURL := e.publicBaseURL()
	fmt.Printf("%s[Fox]%s OpenAPI UI:   %s%s/docs%s\n",
		colorCyan, colorReset, colorGreen, baseURL, colorReset)
	fmt.Printf("%s[Fox]%s OpenAPI JSON: %s%s/openapi.json%s\n\n",
		colorCyan, colorReset, colorGreen, baseURL, colorReset)
}

func (e *Engine) publicBaseURL() string {
	addr := e.cfg.Addr
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			return "http://localhost" + addr
		}
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// getHandlerName 从 HandlerFunc 获取函数名
func (e *Engine) getHandlerName(h HandlerFunc) string {
	if h == nil {
		return "<nil>"
	}

	fn := runtime.FuncForPC(reflect.ValueOf(h).Pointer())
	if fn == nil {
		return "<unknown>"
	}
	name := fn.Name()

	// 去掉包路径，保留包名.函数名
	if idx := strings.LastIndex(name, "/"); idx != -1 {
		name = name[idx+1:]
	}

	// 匿名函数按包名分组重新编号：main.func1
	if strings.Contains(name, ".func") {
		// 提取包名（第一个点之前的部分）
		parts := strings.Split(name, ".")
		if len(parts) >= 1 {
			pkgName := parts[0]
			e.anonFuncCounters[pkgName]++
			return fmt.Sprintf("%s.func%d", pkgName, e.anonFuncCounters[pkgName])
		}
	}

	return name
}
