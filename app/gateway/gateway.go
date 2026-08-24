// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"esx/app/gateway/internal/config"
	"esx/app/gateway/internal/handler"
	"esx/app/gateway/internal/httpxconfig"
	gatewaymiddleware "esx/app/gateway/internal/middleware"
	"esx/app/gateway/internal/svc"
	"esx/pkg/middleware"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "etc/gateway.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	server := rest.MustNewServer(c.RestConf,
		rest.WithUnauthorizedCallback(httpxconfig.Unauthorized),
		rest.WithCors(corsOrigins()...),
	)
	defer server.Stop()

	httpxconfig.ConfigureErrors()

	ctx := svc.NewServiceContext(c)
	server.Use(corsRestMiddleware())
	server.Use(gatewaymiddleware.NewTraceMiddleware().Handle)
	handler.RegisterHandlers(server, ctx)

	fmt.Printf("Starting server at %s:%d...\n", c.RestConf.Host, c.RestConf.Port)
	server.Start()
}

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

func corsRestMiddleware() rest.Middleware {
	cfg := middleware.DefaultCORSConfig
	cfg.AllowOrigins = corsOrigins()
	handler := middleware.CORSMiddleware(cfg)
	return func(next http.HandlerFunc) http.HandlerFunc {
		return handler(next).ServeHTTP
	}
}
