package main

import (
	native "esx/kitex_gen/recommend/recommendservice"
	"esx/pkg/lifecycle"
	"flag"
	"fmt"

	"esx/app/recommend/rpc/internal/config"
	"esx/app/recommend/rpc/internal/server"
	"esx/app/recommend/rpc/internal/svc"

	conf "esx/pkg/configx"
	"esx/pkg/logging"

	"esx/pkg/rpcx"
)

var configFile = flag.String("f", "etc/recommend.yaml", "the config file")

func main() {
	defer lifecycle.CloseResources()
	flag.Parse()
	defer rpcx.CloseAllClients()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	ctx, err := svc.NewServiceContext(c)
	if err != nil {
		logging.Must(err)
	}
	defer func() {
		if err := ctx.Close(); err != nil {
			logging.Error(err)
		}
	}()

	s := native.NewServer(server.NewRecommendServiceServer(ctx), rpcx.ServerOptions(c.RpcServerConf, c.InternalSecret)...)
	defer func() { _ = s.Stop() }()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	if err := s.Run(); err != nil {
		panic(err)
	}
}
