package middleware

import (
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/route"
)

// performHertz 在不经网络的 Hertz 引擎上执行一次请求：middleware 挂在 path 上，handler 作为最终处理器。
func performHertz(middleware, handler app.HandlerFunc, method, path string, headers ...ut.Header) *ut.ResponseRecorder {
	engine := route.NewEngine(config.NewOptions(nil))
	engine.Handle(method, path, middleware, handler)
	return ut.PerformRequest(engine, method, path, nil, headers...)
}
