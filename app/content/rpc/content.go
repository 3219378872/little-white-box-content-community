package main

import (
	"context"
	"errors"
	"esx/app/content/rpc/internal/config"
	"esx/app/content/rpc/internal/server"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"flag"
	"fmt"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/content.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	ctx := svc.NewServiceContext(c)
	defer func() {
		if err := ctx.Close(); err != nil {
			logx.Errorw("close content service dependencies", logx.Field("err", err.Error()))
		}
	}()
	relayCtx, cancelRelay := context.WithCancel(context.Background())
	defer cancelRelay()
	go func() {
		if err := ctx.RunOutboxRelay(relayCtx); err != nil && !errors.Is(err, context.Canceled) {
			logx.Errorw("content outbox relay stopped", logx.Field("err", err.Error()))
		}
	}()

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		pb.RegisterContentServiceServer(grpcServer, server.NewContentServiceServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
