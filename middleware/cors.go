package middleware

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/surge-go/fox"
)

const defaultCORSMaxAge = 24 * time.Hour

// CORSConfig 表示 CORS 中间件配置。
type CORSConfig struct {
	// AllowOrigins 允许的来源列表，支持 "*" 和 "*.example.com"。
	AllowOrigins []string
	// AllowMethods 允许的 HTTP 方法。
	AllowMethods []string
	// AllowHeaders 允许的请求头。
	AllowHeaders []string
	// ExposeHeaders 暴露给浏览器读取的响应头。
	ExposeHeaders []string
	// AllowCredentials 是否允许跨域请求携带 Cookie、Authorization 等凭证。
	AllowCredentials bool
	// MaxAge 表示预检请求缓存时间，0 使用默认值。
	MaxAge time.Duration
}

// DefaultCORSConfig 返回 CORS 中间件默认配置。
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodHead,
			http.MethodOptions,
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
		},
		MaxAge: defaultCORSMaxAge,
	}
}

// CORS 返回使用默认配置的 CORS 中间件。
func CORS() fox.HandlerFunc {
	cfg := DefaultCORSConfig()
	return CORSWithConfig(cfg)
}

// CORSWithConfig 返回使用自定义配置的 CORS 中间件。
func CORSWithConfig(cfg CORSConfig) fox.HandlerFunc {
	cfg = normalizeCORSConfig(cfg)
	allowAllOrigins := len(cfg.AllowOrigins) == 1 && cfg.AllowOrigins[0] == "*"
	allowMethods := strings.Join(cfg.AllowMethods, ", ")
	allowHeaders := strings.Join(cfg.AllowHeaders, ", ")
	exposeHeaders := strings.Join(cfg.ExposeHeaders, ", ")
	maxAge := strconv.FormatInt(int64(cfg.MaxAge/time.Second), 10)

	return func(c *fox.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if allowAllOrigins {
				if cfg.AllowCredentials {
					c.SetHeader("Access-Control-Allow-Origin", origin)
					addVaryHeader(c, "Origin")
				} else {
					c.SetHeader("Access-Control-Allow-Origin", "*")
				}
			} else if isCORSOriginAllowed(origin, cfg.AllowOrigins) {
				c.SetHeader("Access-Control-Allow-Origin", origin)
				addVaryHeader(c, "Origin")
			}
		}

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

		if cfg.AllowCredentials {
			c.SetHeader("Access-Control-Allow-Credentials", "true")
		}
		if exposeHeaders != "" {
			c.SetHeader("Access-Control-Expose-Headers", exposeHeaders)
		}
		c.Next()
	}
}

func normalizeCORSConfig(cfg CORSConfig) CORSConfig {
	defaults := DefaultCORSConfig()
	if len(cfg.AllowOrigins) == 0 {
		cfg.AllowOrigins = defaults.AllowOrigins
	}
	if len(cfg.AllowMethods) == 0 {
		cfg.AllowMethods = defaults.AllowMethods
	}
	if len(cfg.AllowHeaders) == 0 {
		cfg.AllowHeaders = defaults.AllowHeaders
	}
	if cfg.MaxAge == 0 {
		cfg.MaxAge = defaults.MaxAge
	}

	cfg.AllowOrigins = cloneAndCleanStrings(cfg.AllowOrigins)
	cfg.AllowMethods = cloneAndCleanStrings(cfg.AllowMethods)
	cfg.AllowHeaders = cloneAndCleanStrings(cfg.AllowHeaders)
	cfg.ExposeHeaders = cloneAndCleanStrings(cfg.ExposeHeaders)
	return cfg
}

func cloneAndCleanStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func isCORSOriginAllowed(origin string, allowOrigins []string) bool {
	originHost, ok := corsOriginHostname(origin)
	for _, allowed := range allowOrigins {
		if allowed == origin {
			return true
		}
		if ok && strings.HasPrefix(allowed, "*.") {
			domain := strings.ToLower(strings.TrimSuffix(allowed[2:], "."))
			if originHost != domain && strings.HasSuffix(originHost, "."+domain) {
				return true
			}
		}
	}
	return false
}

func corsOriginHostname(origin string) (string, bool) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	return host, host != ""
}

func addVaryHeader(c *fox.Context, value string) {
	current := c.GetResponseHeader("Vary")
	if current == "" {
		c.SetHeader("Vary", value)
		return
	}
	for _, item := range strings.Split(current, ",") {
		if strings.EqualFold(strings.TrimSpace(item), value) {
			return
		}
	}
	c.SetHeader("Vary", current+", "+value)
}
