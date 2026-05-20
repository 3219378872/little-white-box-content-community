package main

import (
	"context"
	"flag"
	"fmt"

	"cleanupx"
	"esx/app/content/mq/cleanup/internal/config"
	"esx/app/content/mq/cleanup/internal/mqs"
	"esx/app/content/mq/cleanup/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

var configFile = flag.String("f", "etc/content-cleanup.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	svcCtx := svc.NewServiceContext(c)

	cleanupConsumer, err := mqs.NewCleanupConsumer(svcCtx)
	if err != nil {
		logx.Must(err)
	}
	if err := cleanupConsumer.Start(); err != nil {
		logx.Must(err)
	}
	defer cleanupx.Shutdown(logx.WithContext(context.Background()), "content-cleanup consumer", cleanupConsumer.Shutdown)

	fmt.Println("Content cleanup MQ consumer started, subscribing post-delete...")
	select {}
}
