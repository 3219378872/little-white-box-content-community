package main

import (
	native "esx/kitex_gen/message/messageservice"
	"esx/pkg/lifecycle"
	"flag"
	"fmt"

	"esx/app/message/rpc/internal/config"
	"esx/app/message/rpc/internal/server"
	"esx/app/message/rpc/internal/svc"

	conf "esx/pkg/configx"

	"esx/pkg/rpcx"
)

var configFile = flag.String("f", "etc/message.yaml", "the config file")

// main 启动私信与通知 RPC 服务。
func main() {
	defer lifecycle.CloseResources()
	flag.Parse()
	defer rpcx.CloseAllClients()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	ctx := svc.NewServiceContext(c)

	s := native.NewServer(server.NewMessageServiceServer(ctx), rpcx.ServerOptions(c.RpcServerConf, c.InternalSecret)...)
	defer func() { _ = s.Stop() }()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	if err := s.Run(); err != nil {
		panic(err)
	}
}
