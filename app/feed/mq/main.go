package main

import (
	"context"
	"esx/pkg/rpcx"
	"flag"
	"fmt"

	"esx/app/feed/mq/internal/config"
	"esx/app/feed/mq/internal/mqs"
	"esx/app/feed/mq/internal/svc"
	"esx/pkg/cleanupx"

	conf "esx/pkg/configx"
	proc "esx/pkg/lifecycle"
	logx "esx/pkg/logging"
)

var configFile = flag.String("f", "etc/feed-consumer.yaml", "config file")

func main() {
	defer rpcx.CloseAllClients()
	defer proc.CloseResources()
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	svcCtx := svc.NewServiceContext(c)

	postConsumer, err := mqs.NewPostPublishConsumer(svcCtx)
	if err != nil {
		logx.Must(err)
	}
	if err := postConsumer.Start(); err != nil {
		logx.Must(err)
	}
	defer cleanupx.Shutdown(logx.WithContext(context.Background()), "feed post-publish consumer", postConsumer.Shutdown)

	fmt.Println("Feed MQ consumer started, subscribing post-create...")
	<-proc.Done()
}
