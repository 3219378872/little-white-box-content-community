package middleware

import (
	"context"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/assert"
)

// noopHandler 是 CORS 测试的最终处理器，只返回默认 200。
func noopHandler(context.Context, *app.RequestContext) {}

func TestHertzCORS_WhitelistWithCredentials(t *testing.T) {
	mw := HertzCORS(CORSConfig{
		AllowOrigins:     []string{"https://example.com"},
		AllowMethods:     []string{"GET", "POST"},
		AllowCredentials: true,
	})

	// 合法 origin
	resp := performHertz(mw, noopHandler, http.MethodGet, "/", ut.Header{Key: "Origin", Value: "https://example.com"}).Result()
	assert.Equal(t, "https://example.com", string(resp.Header.Peek("Access-Control-Allow-Origin")))
	assert.Equal(t, "true", string(resp.Header.Peek("Access-Control-Allow-Credentials")))

	// 非法 origin 不应设置 Access-Control-Allow-Origin
	resp = performHertz(mw, noopHandler, http.MethodGet, "/", ut.Header{Key: "Origin", Value: "https://evil.com"}).Result()
	assert.Empty(t, resp.Header.Peek("Access-Control-Allow-Origin"))
}

func TestHertzCORS_NoCredentialsWildcard(t *testing.T) {
	mw := HertzCORS(CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET"},
		AllowCredentials: false,
	})

	resp := performHertz(mw, noopHandler, http.MethodGet, "/", ut.Header{Key: "Origin", Value: "https://any.com"}).Result()
	assert.Equal(t, "*", string(resp.Header.Peek("Access-Control-Allow-Origin")))
}

func TestHertzCORS_CredentialsNoWildcard(t *testing.T) {
	mw := HertzCORS(CORSConfig{
		AllowOrigins:     []string{},
		AllowMethods:     []string{"GET"},
		AllowCredentials: true,
	})

	resp := performHertz(mw, noopHandler, http.MethodGet, "/", ut.Header{Key: "Origin", Value: "https://any.com"}).Result()
	assert.Empty(t, resp.Header.Peek("Access-Control-Allow-Origin"))
}

func TestHertzCORS_PreflightShortCircuitsAndNoOriginPassesThrough(t *testing.T) {
	mw := HertzCORS(DefaultCORSConfig)
	called := false
	handler := func(context.Context, *app.RequestContext) { called = true }

	resp := performHertz(mw, handler, http.MethodOptions, "/", ut.Header{Key: "Origin", Value: "https://any.com"}).Result()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
	assert.Equal(t, "86400", string(resp.Header.Peek("Access-Control-Max-Age")))
	assert.False(t, called)

	resp = performHertz(mw, handler, http.MethodGet, "/").Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	assert.Empty(t, resp.Header.Peek("Vary"))
	assert.True(t, called)
}
