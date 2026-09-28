package main

import (
	"context"
	native "esx/kitex_gen/interaction/interactionservice"
	"esx/pkg/lifecycle"
	"flag"
	"fmt"

	"esx/app/interaction/rpc/internal/config"
	"esx/app/interaction/rpc/internal/server"
	"esx/app/interaction/rpc/internal/svc"

	"esx/pkg/outboxx"

	conf "esx/pkg/configx"
	logx "esx/pkg/logging"

	"esx/pkg/rpcx"
)

var configFile = flag.String("f", "etc/interaction.yaml", "the config file")

func main() {
	defer lifecycle.CloseResources()
	flag.Parse()
	defer rpcx.CloseAllClients()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	ctx := svc.NewServiceContext(c)
	defer func() {
		if err := ctx.Close(); err != nil {
			logx.Errorw("close interaction service dependencies", logx.Field("err", err.Error()))
		}
	}()
	relay := outboxx.StartRelay(context.Background(), ctx.OutboxRelay)
	defer func() {
		if err := relay.Stop(); err != nil {
			logx.Errorw("interaction outbox relay stopped", logx.Field("err", err.Error()))
		}
	}()

	s := native.NewServer(server.NewInteractionServiceServer(ctx), rpcx.ServerOptions(c.RpcServerConf, c.InternalSecret)...)
	defer func() { _ = s.Stop() }()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	if err := s.Run(); err != nil {
		panic(err)
	}
}
