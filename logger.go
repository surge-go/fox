package fox

import (
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
)

const colorReset = "\033[0m"
const colorRed = "\033[31m"
const colorBlue = "\033[34m"
const colorCyan = "\033[36m"
const colorYellow = "\033[33m"
const colorGreen = "\033[32m"
const colorMagenta = "\033[35m"
const colorWhite = "\033[37m"

// RouteInfo 是启动路由表打印使用的路由快照。
type RouteInfo struct {
	Method  string
	Path    string
	Handler string
	Hidden  bool
}

// printRoutes 打印当前 engine 已注册的路由表。
func (e *Engine) printRoutes() {
	routes := e.routeSnapshot()
	if len(routes) == 0 {
		return
	}

	e.printBanner()
	for _, route := range routes {
		if route.Hidden {
			continue
		}
		color := methodColor(route.Method)
		fmt.Printf("%s[Fox-debug]%s %s%-7s%s %-30s --> %s\n",
			colorCyan, colorReset, color, route.Method, colorReset, route.Path, route.Handler)
	}
	fmt.Println()
}

// routeSnapshot 返回当前 engine 记录的路由快照。
func (e *Engine) routeSnapshot() []RouteInfo {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()

	if len(e.routes) == 0 {
		return nil
	}

	routes := make([]RouteInfo, len(e.routes))
	copy(routes, e.routes)
	return routes
}

// printBanner 打印启动横幅和基础运行信息。
func (e *Engine) printBanner() {
	runtimeText := runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH
	logo := []string{
		"███████╗ ██████╗ ██╗  ██╗",
		"██╔════╝██╔═══██╗╚██╗██╔╝",
		"█████╗  ██║   ██║ ╚███╔╝ ",
		"██╔══╝  ██║   ██║ ██╔██╗ ",
		"██║     ╚██████╔╝██╔╝ ██╗",
		"╚═╝      ╚═════╝ ╚═╝  ╚═╝",
	}
	info := []struct {
		label string
		value string
		color string
	}{
		{label: "Status", value: "HTTP server is ready", color: colorGreen},
		{label: "Mode", value: string(e.mode), color: colorMagenta},
		{label: "Address", value: e.publicBaseURL(), color: colorGreen},
		{label: "Runtime", value: runtimeText, color: colorWhite},
	}

	fmt.Println()
	for i, line := range logo {
		fmt.Printf("%s%s%s", colorCyan, line, colorReset)
		if i < len(info) {
			item := info[i]
			fmt.Printf("    %s%-8s%s %s%s%s",
				colorWhite, item.label, colorReset,
				item.color, formatBannerValue(item.value, 42), colorReset,
			)
		}
		fmt.Println()
	}
	fmt.Printf("%s%s%s\n\n", colorCyan, strings.Repeat("─", 78), colorReset)
}

func formatBannerValue(value string, max int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "-"
	}
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

// publicBaseURL 返回用于打印的服务访问地址。
func (e *Engine) publicBaseURL() string {
	if e == nil || e.cfg == nil {
		return ""
	}

	scheme := "http"
	if e.cfg.TLS != nil {
		scheme = "https"
	}

	addr := e.cfg.Addr
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			return scheme + "://localhost" + addr
		}
		return scheme + "://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "localhost"
	}
	return scheme + "://" + net.JoinHostPort(host, port)
}

// methodColor 返回 HTTP 方法对应的 ANSI 颜色。
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

// displayHandlerName 精简函数名，保留对启动路由表最有用的部分。
func displayHandlerName(name string) string {
	if name == "" {
		return "<unknown>"
	}
	if idx := strings.LastIndex(name, "/"); idx != -1 {
		name = name[idx+1:]
	}
	if idx := strings.Index(name, "["); idx != -1 {
		name = name[:idx]
	}
	name = strings.TrimSuffix(name, "-fm")
	parts := strings.Split(name, ".")
	if len(parts) >= 3 && strings.HasPrefix(parts[len(parts)-1], "func") {
		return parts[0] + "." + parts[len(parts)-1]
	}
	return name
}

func displayFunctionName(fn any) string {
	return joinFileAndHandlerName(handlerFileName(fn), displayHandlerName(functionName(fn)))
}

func displayRouteHandlerName(fn any) string {
	name := displayHandlerName(functionName(fn))
	return joinFileAndHandlerName(handlerFileName(fn), name)
}

func joinFileAndHandlerName(file, name string) string {
	if name == "" {
		name = "<unknown>"
	}
	if file == "" || isGeneratedFileName(file) {
		return name
	}
	if strings.HasPrefix(name, file+".") {
		return name
	}
	return file + ":" + name
}

func handlerPointer(fn any) uintptr {
	if fn == nil {
		return 0
	}
	value := reflect.ValueOf(fn)
	if value.Kind() != reflect.Func {
		return 0
	}
	return value.Pointer()
}

func handlerFileName(fn any) string {
	pointer := handlerPointer(fn)
	if pointer == 0 {
		return ""
	}
	runtimeFn := runtime.FuncForPC(pointer)
	if runtimeFn == nil {
		return ""
	}
	file, _ := runtimeFn.FileLine(pointer)
	if file == "" {
		return ""
	}
	base := filepath.Base(file)
	if isGeneratedFileName(base) {
		return ""
	}
	return strings.TrimSuffix(base, ".go")
}

func isGeneratedFileName(name string) bool {
	return strings.HasPrefix(name, "<") && strings.HasSuffix(name, ">")
}

func functionName(fn any) string {
	if fn == nil {
		return ""
	}
	pointer := handlerPointer(fn)
	if pointer == 0 {
		return ""
	}
	runtimeFn := runtime.FuncForPC(pointer)
	if runtimeFn == nil {
		return ""
	}
	return runtimeFn.Name()
}
