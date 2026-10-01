package main

import (
	"context"
	"flag"
	"fmt"

	"esx/app/ad/rpc/internal/config"
	"esx/app/ad/rpc/internal/server"
	"esx/app/ad/rpc/internal/svc"
	native "esx/kitex_gen/ad/adservice"
	"esx/pkg/lifecycle"
	"esx/pkg/outboxx"
	"esx/pkg/rpcx"

	conf "esx/pkg/configx"
	logx "esx/pkg/logging"
)

var configFile = flag.String("f", "etc/ad.yaml", "the config file")

func main() {
	defer lifecycle.CloseResources()
	flag.Parse()
	defer rpcx.CloseAllClients()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	if c.Assets.AccessKey == "" || c.Assets.SecretKey == "" {
		panic("ad-rpc: S3_ACCESS_KEY and S3_SECRET_KEY must be set")
	}
	ctx := svc.NewServiceContext(c)
	defer func() {
		if err := ctx.Close(); err != nil {
			logx.Errorw("close ad service dependencies", logx.Field("err", err.Error()))
		}
	}()
	if ctx.OutboxRelay != nil {
		relay := outboxx.StartRelay(context.Background(), ctx.OutboxRelay)
		defer func() {
			if err := relay.Stop(); err != nil {
				logx.Errorw("ad outbox relay stopped", logx.Field("err", err.Error()))
			}
		}()
	}

	s := native.NewServer(server.NewAdServiceServer(ctx), rpcx.ServerOptions(c.RpcServerConf, c.InternalSecret)...)
	defer func() { _ = s.Stop() }()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	if err := s.Run(); err != nil {
		panic(err)
	}
}
