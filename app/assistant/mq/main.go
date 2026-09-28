package main

import (
	"context"
	"esx/pkg/rpcx"
	"flag"
	"fmt"

	"esx/app/assistant/mq/internal/config"
	"esx/app/assistant/mq/internal/mqs"
	"esx/app/assistant/mq/internal/svc"
	"esx/pkg/cleanupx"

	conf "esx/pkg/configx"
	proc "esx/pkg/lifecycle"
	logx "esx/pkg/logging"
)

var configFile = flag.String("f", "etc/watch-consumer.yaml", "config file")

func main() {
	defer rpcx.CloseAllClients()
	defer proc.CloseResources()
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	c.MustSetUp()

	svcCtx, err := svc.NewServiceContext(c)
	if err != nil {
		logx.Must(err)
	}

	watchConsumer, err := mqs.NewWatchConsumer(svcCtx)
	if err != nil {
		logx.Must(err)
	}
	if err := watchConsumer.Start(); err != nil {
		logx.Must(err)
	}
	defer cleanupx.Shutdown(logx.WithContext(context.Background()), "watch matcher", watchConsumer.Shutdown)

	fmt.Println("Assistant watch matcher started, subscribing post lifecycle topics...")
	<-proc.Done()
}
