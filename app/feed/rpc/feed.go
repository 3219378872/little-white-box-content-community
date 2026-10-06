package main

import (
	native "esx/kitex_gen/feed/feedservice"
	"esx/pkg/lifecycle"
	"flag"
	"fmt"

	"esx/app/feed/rpc/internal/config"
	"esx/app/feed/rpc/internal/server"
	"esx/app/feed/rpc/internal/svc"

	conf "esx/pkg/configx"

	"esx/pkg/rpcx"
)

var configFile = flag.String("f", "etc/feed.yaml", "the config file")

// main 启动 Feed RPC 服务。
func main() {
	defer lifecycle.CloseResources()
	flag.Parse()
	defer rpcx.CloseAllClients()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	ctx := svc.NewServiceContext(c)

	s := native.NewServer(server.NewFeedServiceServer(ctx), rpcx.ServerOptions(c.RpcServerConf, c.InternalSecret)...)
	defer func() { _ = s.Stop() }()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	if err := s.Run(); err != nil {
		panic(err)
	}
}
