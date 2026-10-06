package main

import (
	"context"
	native "esx/kitex_gen/search/searchservice"
	"esx/pkg/lifecycle"
	"flag"
	"fmt"

	"esx/app/search/rpc/internal/config"
	"esx/app/search/rpc/internal/server"
	"esx/app/search/rpc/internal/svc"

	conf "esx/pkg/configx"
	"esx/pkg/logging"

	"esx/pkg/rpcx"
)

var configFile = flag.String("f", "etc/search.yaml", "the config file")

// main 启动搜索 RPC 服务；ES 健康检查在装配依赖时完成。
func main() {
	defer lifecycle.CloseResources()
	flag.Parse()
	defer rpcx.CloseAllClients()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	ctx := svc.NewServiceContext(c)

	s := native.NewServer(server.NewSearchServiceServer(ctx), rpcx.ServerOptions(c.RpcServerConf, c.InternalSecret)...)
	defer func() { _ = s.Stop() }()

	logging.WithContext(context.Background()).Infow("search rpc ready",
		logging.Field("listen_on", c.ListenOn), logging.Field("index", c.ES.Index))
	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	if err := s.Run(); err != nil {
		panic(err)
	}
}
