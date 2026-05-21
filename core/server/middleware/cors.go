package middleware

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/surge-go/fox/core/server"
)

// CORSConfig CORS 中间件配置
type CORSConfig struct {
	// AllowOrigins 允许的源列表，支持通配符 "*"
	AllowOrigins []string
	// AllowMethods 允许的 HTTP 方法
	AllowMethods []string
	// AllowHeaders 允许的请求头
	AllowHeaders []string
	// ExposeHeaders 暴露给客户端的响应头
	ExposeHeaders []string
	// AllowCredentials 是否允许携带凭证（Cookie）
	AllowCredentials bool
	// MaxAge 预检请求缓存时间（秒）
	MaxAge int
}

// CORS 返回 CORS 中间件
//
// 处理跨域资源共享（CORS）请求，支持简单请求和预检请求。
//
// 示例：
//
//	// 允许所有源
//	srv.Use(middleware.CORS(nil))
//
//	// 自定义配置
//	srv.Use(middleware.CORS(&middleware.CORSConfig{
//	    AllowOrigins:     []string{"https://example.com", "https://app.example.com"},
//	    AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
//	    AllowHeaders:     []string{"Authorization", "Content-Type"},
//	    AllowCredentials: true,
//	    MaxAge:           3600,
//	}))
func CORS(cfg *CORSConfig) server.HandlerFunc {
	if cfg == nil {
		cfg = &CORSConfig{
			AllowOrigins: []string{"*"},
			AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
			AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization"},
			MaxAge:       86400, // 24 小时
		}
	}

	// 默认值
	if len(cfg.AllowOrigins) == 0 {
		cfg.AllowOrigins = []string{"*"}
	}
	if len(cfg.AllowMethods) == 0 {
		cfg.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	}
	if len(cfg.AllowHeaders) == 0 {
		cfg.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization"}
	}
	if cfg.MaxAge == 0 {
		cfg.MaxAge = 86400
	}

	allowAllOrigins := len(cfg.AllowOrigins) == 1 && cfg.AllowOrigins[0] == "*"
	allowMethods := strings.Join(cfg.AllowMethods, ", ")
	allowHeaders := strings.Join(cfg.AllowHeaders, ", ")
	exposeHeaders := strings.Join(cfg.ExposeHeaders, ", ")
	maxAge := strconv.Itoa(cfg.MaxAge)

	return func(c *server.Context) {
		origin := c.GetHeader("Origin")

		// 检查是否允许该源
		if origin != "" {
			if allowAllOrigins {
				if cfg.AllowCredentials {
					c.SetHeader("Access-Control-Allow-Origin", origin)
					c.SetHeader("Vary", "Origin")
				} else {
					c.SetHeader("Access-Control-Allow-Origin", "*")
				}
			} else if isOriginAllowed(origin, cfg.AllowOrigins) {
				c.SetHeader("Access-Control-Allow-Origin", origin)
				c.SetHeader("Vary", "Origin")
			}
		}

		// 处理预检请求（OPTIONS）
		if c.RawRequest().Method == http.MethodOptions {
			c.SetHeader("Access-Control-Allow-Methods", allowMethods)
			c.SetHeader("Access-Control-Allow-Headers", allowHeaders)
			if cfg.AllowCredentials {
				c.SetHeader("Access-Control-Allow-Credentials", "true")
			}
			c.SetHeader("Access-Control-Max-Age", maxAge)
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		// 处理实际请求
		if cfg.AllowCredentials {
			c.SetHeader("Access-Control-Allow-Credentials", "true")
		}
		if exposeHeaders != "" {
			c.SetHeader("Access-Control-Expose-Headers", exposeHeaders)
		}

		c.Next()
	}
}

// isOriginAllowed 检查源是否在允许列表中
func isOriginAllowed(origin string, allowOrigins []string) bool {
	originHost, ok := originHostname(origin)
	for _, allowed := range allowOrigins {
		if allowed == origin {
			return true
		}
		// 支持通配符子域名，如 "*.example.com"
		if ok && strings.HasPrefix(allowed, "*.") {
			domain := strings.ToLower(strings.TrimSuffix(allowed[2:], "."))
			if originHost != domain && strings.HasSuffix(originHost, "."+domain) {
				return true
			}
		}
	}
	return false
}

func originHostname(origin string) (string, bool) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	return host, host != ""
}
