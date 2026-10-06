package main

import (
	"esx/app/gateway/internal/config"
	"esx/app/gateway/internal/handler"
	gatewaymiddleware "esx/app/gateway/internal/middleware"
	"esx/app/gateway/internal/svc"
	conf "esx/pkg/configx"
	"esx/pkg/lifecycle"
	"esx/pkg/middleware"
	"esx/pkg/rpcx"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/cloudwego/hertz/pkg/app/server"
)

var configFile = flag.String("f", "etc/gateway.yaml", "the config file")

// main 加载配置、装配下游 RPC 客户端后启动 Hertz 网关；退出时按 defer 顺序关闭客户端与共享资源。
func main() {
	defer lifecycle.CloseResources()
	flag.Parse()
	defer rpcx.CloseAllClients()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	c.RestConf.MustSetUp()
	h := newGateway(c, svc.NewServiceContext(c))
	fmt.Printf("Starting server at %s:%d...\n", c.RestConf.Host, c.RestConf.Port)
	h.Spin()
}

// newGateway 构造 Hertz 服务器：上传路由需要流式请求体并跳过预解析 multipart，
// 请求体上限略高于 100MiB 的视频上限以便返回 FileTooLarge 而非连接错误。
func newGateway(c config.Config, ctx *svc.ServiceContext) *server.Hertz {
	h := server.New(server.WithHostPorts(net.JoinHostPort(c.RestConf.Host, fmt.Sprint(c.RestConf.Port))), server.WithStreamBody(true), server.WithDisablePreParseMultipartForm(true), server.WithMaxRequestBodySize(101<<20), server.WithSenseClientDisconnection(true))
	cors := middleware.DefaultCORSConfig
	cors.AllowOrigins = corsOrigins()
	h.Use(gatewaymiddleware.NewTraceMiddleware().Hertz, gatewaymiddleware.NewSafeAccessLogMiddleware().Hertz, gatewaymiddleware.Recovery, middleware.HertzCORS(cors))
	handler.RegisterHandlers(h, ctx)
	return h
}

// corsOrigins 默认放行本地联调端口；GATEWAY_CORS_ORIGINS 非空时整体替换默认列表。
func corsOrigins() []string {
	origins := []string{
		"http://localhost:3000", "http://127.0.0.1:3000",
		"http://localhost:3001", "http://127.0.0.1:3001",
		"http://localhost:3002", "http://127.0.0.1:3002",
	}
	if raw := strings.TrimSpace(os.Getenv("GATEWAY_CORS_ORIGINS")); raw != "" {
		origins = nil
		for origin := range strings.SplitSeq(raw, ",") {
			origin = strings.TrimSpace(origin)
			if origin != "" {
				origins = append(origins, origin)
			}
		}
	}
	return origins
}
