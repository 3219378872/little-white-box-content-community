package middleware

import "net/http"

// CORSConfig 是 HertzCORS 的配置：来源白名单、允许的方法与头、是否携带凭证及预检缓存时长。
type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
}

// DefaultCORSConfig 默认 CORS 配置
var DefaultCORSConfig = CORSConfig{
	AllowOrigins: []string{}, // 禁止默认通配，由调用方显式配置白名单
	AllowMethods: []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodOptions,
		http.MethodPatch,
	},
	AllowHeaders: []string{
		"Content-Type",
		"Authorization",
		"X-Requested-With",
		"X-Token",
	},
	ExposeHeaders:    []string{},
	AllowCredentials: true,
	MaxAge:           86400,
}
