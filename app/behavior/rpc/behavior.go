package main

import (
	"context"
	"flag"
	"fmt"

	"cleanupx"
	"esx/app/behavior/rpc/internal/config"
	"esx/app/behavior/rpc/internal/server"
	"esx/app/behavior/rpc/internal/svc"
	"esx/app/behavior/rpc/xiaobaihe/behavior/pb"
	"interceptor"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/behavior.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	ctx := svc.NewServiceContext(c)
	logger := logx.WithContext(context.Background())
	defer cleanupx.Shutdown(logger, "behavior producer", ctx.Close)

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		pb.RegisterBehaviorServiceServer(grpcServer, server.NewBehaviorServiceServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	s.AddUnaryInterceptors(interceptor.InternalAuthUnaryServerInterceptor(c.InternalSecret))
	defer s.Stop()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
