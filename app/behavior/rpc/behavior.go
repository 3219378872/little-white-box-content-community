package main

import (
	"context"
	native "esx/kitex_gen/behavior/behaviorservice"
	"esx/pkg/lifecycle"
	"flag"
	"fmt"

	"esx/app/behavior/rpc/internal/config"
	"esx/app/behavior/rpc/internal/server"
	"esx/app/behavior/rpc/internal/svc"

	"esx/pkg/cleanupx"

	conf "esx/pkg/configx"
	"esx/pkg/logging"

	"esx/pkg/rpcx"
)

var configFile = flag.String("f", "etc/behavior.yaml", "the config file")

// main 启动行为上报 RPC 服务，退出时关闭 MQ 生产者以刷出未发送的消息。
func main() {
	defer lifecycle.CloseResources()
	flag.Parse()
	defer rpcx.CloseAllClients()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	ctx := svc.NewServiceContext(c)
	logger := logging.WithContext(context.Background())
	defer cleanupx.Shutdown(logger, "behavior producer", ctx.Close)

	s := native.NewServer(server.NewBehaviorServiceServer(ctx), rpcx.ServerOptions(c.RpcServerConf, c.InternalSecret)...)
	defer func() { _ = s.Stop() }()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	if err := s.Run(); err != nil {
		panic(err)
	}
}
