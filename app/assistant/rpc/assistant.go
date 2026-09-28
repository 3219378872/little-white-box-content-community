package main

import (
	native "esx/kitex_gen/assistant/assistantservice"
	"esx/pkg/lifecycle"
	"flag"
	"fmt"

	"esx/app/assistant/rpc/internal/config"
	"esx/app/assistant/rpc/internal/server"
	"esx/app/assistant/rpc/internal/svc"

	conf "esx/pkg/configx"

	"esx/pkg/rpcx"
)

var configFile = flag.String("f", "etc/assistant.yaml", "the config file")

func main() {
	defer lifecycle.CloseResources()
	flag.Parse()
	defer rpcx.CloseAllClients()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	ctx := svc.NewServiceContext(c)

	s := native.NewServer(server.NewAssistantServiceServer(ctx), rpcx.ServerOptions(c.RpcServerConf, c.InternalSecret)...)
	defer func() { _ = s.Stop() }()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	if err := s.Run(); err != nil {
		panic(err)
	}
}
