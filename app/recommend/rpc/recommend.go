package main

import (
	"flag"
	"fmt"

	"esx/app/recommend/rpc/internal/config"
	"esx/app/recommend/rpc/internal/server"
	"esx/app/recommend/rpc/internal/svc"
	"esx/app/recommend/rpc/xiaobaihe/recommend/pb"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"interceptor"
)

var configFile = flag.String("f", "etc/recommend.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	ctx, err := svc.NewServiceContext(c)
	if err != nil {
		logx.Must(err)
	}
	defer func() {
		if err := ctx.Close(); err != nil {
			logx.Error(err)
		}
	}()

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		pb.RegisterRecommendServiceServer(grpcServer, server.NewRecommendServiceServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	s.AddUnaryInterceptors(interceptor.InternalAuthUnaryServerInterceptor(c.InternalSecret))
	defer s.Stop()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
