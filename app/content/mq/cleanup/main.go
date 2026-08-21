package main

import (
	"context"
	"flag"
	"fmt"

	"esx/app/content/mq/cleanup/internal/config"
	"esx/app/content/mq/cleanup/internal/mqs"
	"esx/app/content/mq/cleanup/internal/svc"
	"esx/pkg/cleanupx"

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

	if svcCtx.CountSyncStore != nil {
		countSyncConsumer, err := mqs.NewCountSyncConsumer(svcCtx)
		if err != nil {
			logx.Must(err)
		}
		if err := countSyncConsumer.Start(); err != nil {
			logx.Must(err)
		}
		defer cleanupx.Shutdown(logx.WithContext(context.Background()), "count-sync consumer", countSyncConsumer.Shutdown)
		defer cleanupx.Shutdown(logx.WithContext(context.Background()), "content cleanup database", svcCtx.Close)
	}

	fmt.Println("Content cleanup MQ consumer started, subscribing post-delete and behavior counts...")
	select {}
}
