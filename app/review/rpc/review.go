package main

import (
	"context"
	"flag"
	"fmt"

	"esx/app/review/rpc/internal/config"
	"esx/app/review/rpc/internal/server"
	"esx/app/review/rpc/internal/svc"
	native "esx/kitex_gen/review/reviewservice"
	"esx/pkg/lifecycle"
	"esx/pkg/outboxx"
	"esx/pkg/rpcx"

	conf "esx/pkg/configx"
	"esx/pkg/logging"
)

var configFile = flag.String("f", "etc/review.yaml", "the config file")

func main() {
	defer lifecycle.CloseResources()
	flag.Parse()
	defer rpcx.CloseAllClients()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	ctx := svc.NewServiceContext(c)
	defer func() {
		if err := ctx.Close(); err != nil {
			logging.Errorw("close review service dependencies", logging.Field("err", err.Error()))
		}
	}()
	if ctx.OutboxRelay != nil {
		relay := outboxx.StartRelay(context.Background(), ctx.OutboxRelay)
		defer func() {
			if err := relay.Stop(); err != nil {
				logging.Errorw("review outbox relay stopped", logging.Field("err", err.Error()))
			}
		}()
	}

	s := native.NewServer(server.NewReviewServiceServer(ctx), rpcx.ServerOptions(c.RpcServerConf, c.InternalSecret)...)
	defer func() { _ = s.Stop() }()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	if err := s.Run(); err != nil {
		panic(err)
	}
}
