// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"

	"errx"

	"gateway/internal/config"
	"gateway/internal/handler"
	"gateway/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

var configFile = flag.String("f", "etc/gateway.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	httpx.SetErrorHandlerCtx(func(ctx context.Context, err error) (int, any) {
		if bizErr, ok := errors.AsType[*errx.BizError](err); ok {
			return bizErr.HTTPStatus(), map[string]any{
				"code":    bizErr.Code,
				"message": bizErr.Message,
			}
		}
		return http.StatusInternalServerError, map[string]any{
			"code":    errx.SystemError,
			"message": errx.GetMsg(errx.SystemError),
		}
	})

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)

	fmt.Printf("Starting server at %s:%d...\n", c.RestConf.Host, c.RestConf.Port)
	server.Start()
}
