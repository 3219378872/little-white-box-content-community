package main

import (
	"flag"
	"fmt"

	"esx/app/media/rpc/internal/config"
	"esx/app/media/rpc/internal/server"
	"esx/app/media/rpc/internal/svc"
	"esx/app/media/rpc/pb/xiaobaihe/media/pb"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/media.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	// 校验 S3 凭据已配置
	if c.S3Storage.AccessKey == "" || c.S3Storage.SecretKey == "" {
		panic("S3_ACCESS_KEY and S3_SECRET_KEY must be set")
	}

	ctx := svc.NewServiceContext(c)

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		pb.RegisterMediaServiceServer(grpcServer, server.NewMediaServiceServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
