package main

import (
	"flag"
	"fmt"

	"esx/app/message/internal/config"
	"esx/app/message/internal/mqs"
	"esx/app/message/internal/server"
	"esx/app/message/internal/svc"
	"esx/app/message/xiaobaihe/message/pb"
	"mqx"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/message.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)
	ctx := svc.NewServiceContext(c)
	var messageConsumer *mqx.Consumer
	if c.MQ.NameServer != "" && c.MQ.Topic != "" {
		var err error
		messageConsumer, err = mqs.NewRocketMQConsumer(ctx)
		if err != nil {
			logx.Must(err)
		}
		if err := messageConsumer.Start(); err != nil {
			logx.Must(err)
		}
		defer messageConsumer.Shutdown()
	}

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		pb.RegisterMessageServiceServer(grpcServer, server.NewMessageServiceServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
