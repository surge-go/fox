package server

import (
	"fmt"
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

	fmt.Println()
	for _, r := range routes {
		color := methodColor(r.Method)
		fmt.Printf("%s[Fox-debug]%s %s%-7s%s %-30s --> %s\n",
			colorCyan, colorReset, color, r.Method, colorReset, r.Path, r.Handler)
	}
	fmt.Println()

	// 打印启动信息
	fmt.Printf("%s[Fox]%s Running in %s%q%s mode.\n",
		colorCyan, colorReset, colorMagenta, e.mode, colorReset)
	fmt.Printf("%s[Fox]%s Go version: %s | OS: %s/%s\n",
		colorCyan, colorReset, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Printf("%s[Fox]%s Listening on %s%s%s\n\n",
		colorCyan, colorReset, colorGreen, e.cfg.Addr, colorReset)
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
