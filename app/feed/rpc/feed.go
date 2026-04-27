package main

import (
	"context"
	"flag"
	"fmt"

	"cleanupx"
	"esx/app/feed/internal/config"
	"esx/app/feed/internal/mqs"
	"esx/app/feed/internal/server"
	"esx/app/feed/internal/svc"
	"esx/app/feed/xiaobaihe/feed/pb"
	"mqx"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/feed.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)
	ctx := svc.NewServiceContext(c)
	var postConsumer *mqx.Consumer
	if c.MQ.NameServer != "" {
		var err error
		postConsumer, err = mqs.NewPostPublishConsumer(ctx)
		if err != nil {
			logx.Must(err)
		}
		if err := postConsumer.Start(); err != nil {
			logx.Must(err)
		}
		defer cleanupx.Shutdown(logx.WithContext(context.Background()), "post publish consumer", postConsumer.Shutdown)
	}

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		pb.RegisterFeedServiceServer(grpcServer, server.NewFeedServiceServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
