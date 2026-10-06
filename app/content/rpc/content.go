package main

import (
	"context"
	native "esx/kitex_gen/content/contentservice"
	"esx/pkg/lifecycle"
	"flag"
	"fmt"

	"esx/app/content/rpc/internal/config"
	"esx/app/content/rpc/internal/server"
	"esx/app/content/rpc/internal/svc"

	"esx/pkg/outboxx"

	conf "esx/pkg/configx"
	"esx/pkg/logging"

	"esx/pkg/rpcx"
)

var configFile = flag.String("f", "etc/content.yaml", "the config file")

// main 启动内容 RPC 服务，并在后台运行 outbox relay 投递帖子生命周期与行为事件。
func main() {
	defer lifecycle.CloseResources()
	flag.Parse()
	defer rpcx.CloseAllClients()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	ctx := svc.NewServiceContext(c)
	defer func() {
		if err := ctx.Close(); err != nil {
			logging.Errorw("close content service dependencies", logging.Field("err", err.Error()))
		}
	}()
	relay := outboxx.StartRelay(context.Background(), ctx.OutboxRelay)
	defer func() {
		if err := relay.Stop(); err != nil {
			logging.Errorw("content outbox relay stopped", logging.Field("err", err.Error()))
		}
	}()

	s := native.NewServer(server.NewContentServiceServer(ctx), rpcx.ServerOptions(c.RpcServerConf, c.InternalSecret)...)
	defer func() { _ = s.Stop() }()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	if err := s.Run(); err != nil {
		panic(err)
	}
}
