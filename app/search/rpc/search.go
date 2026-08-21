package main

import (
	"context"
	"flag"
	"fmt"

	"esx/app/search/rpc/internal/config"
	"esx/app/search/rpc/internal/server"
	"esx/app/search/rpc/internal/svc"
	"esx/app/search/rpc/xiaobaihe/search/pb"
	"esx/pkg/interceptor"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/search.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	ctx := svc.NewServiceContext(c)

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		pb.RegisterSearchServiceServer(grpcServer, server.NewSearchServiceServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	s.AddUnaryInterceptors(interceptor.InternalAuthUnaryServerInterceptor(c.InternalSecret))
	defer s.Stop()

	logx.WithContext(context.Background()).Infow("search rpc ready",
		logx.Field("listen_on", c.ListenOn), logx.Field("index", c.ES.Index))
	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
