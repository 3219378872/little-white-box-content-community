package main

import (
	"context"
	native "esx/kitex_gen/media/mediaservice"
	"esx/pkg/lifecycle"
	"flag"
	"fmt"

	"esx/app/media/rpc/internal/config"
	"esx/app/media/rpc/internal/server"
	"esx/app/media/rpc/internal/svc"

	"esx/pkg/outboxx"

	conf "esx/pkg/configx"
	"esx/pkg/logging"

	"esx/pkg/rpcx"
)

var configFile = flag.String("f", "etc/media.yaml", "the config file")

// main 启动媒体 RPC 服务，并在后台运行 outbox relay 投递清理事件。
func main() {
	defer lifecycle.CloseResources()
	flag.Parse()
	defer rpcx.CloseAllClients()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	// 校验 S3 凭据已配置
	if c.S3Storage.AccessKey == "" || c.S3Storage.SecretKey == "" {
		panic("S3_ACCESS_KEY and S3_SECRET_KEY must be set")
	}

	ctx := svc.NewServiceContext(c)
	defer func() {
		if err := ctx.Close(); err != nil {
			logging.Errorw("close media service dependencies", logging.Field("err", err.Error()))
		}
	}()
	// 未配置 MQ 时 relay 为空操作，事件留在 outbox 表中待配置后投递。
	relay := outboxx.StartRelay(context.Background(), ctx.OutboxRelay)
	defer func() {
		if err := relay.Stop(); err != nil {
			logging.Errorw("media outbox relay stopped", logging.Field("err", err.Error()))
		}
	}()

	s := native.NewServer(server.NewMediaServiceServer(ctx), rpcx.ServerOptions(c.RpcServerConf, c.InternalSecret)...)
	defer func() { _ = s.Stop() }()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	if err := s.Run(); err != nil {
		panic(err)
	}
}
