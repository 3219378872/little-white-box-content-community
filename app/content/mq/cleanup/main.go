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
	"github.com/zeromicro/go-zero/core/proc"
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
	logger := logx.WithContext(context.Background())
	var countSyncShutdown func() error
	defer func() {
		shutdownContentCleanup(logger, countSyncShutdown, cleanupConsumer.Shutdown, svcCtx.Close)
	}()

	if svcCtx.CountSyncStore != nil {
		countSyncConsumer, err := mqs.NewCountSyncConsumer(svcCtx)
		if err != nil {
			logx.Must(err)
		}
		if err := countSyncConsumer.Start(); err != nil {
			logx.Must(err)
		}
		countSyncShutdown = countSyncConsumer.Shutdown
	}

	fmt.Println("Content cleanup MQ consumer started, subscribing post-delete and behavior counts...")
	<-proc.Done()
}

func shutdownContentCleanup(logger logx.Logger, countSync, cleanup, database func() error) {
	cleanupx.Shutdown(logger, "count-sync consumer", countSync)
	cleanupx.Shutdown(logger, "content-cleanup consumer", cleanup)
	cleanupx.Shutdown(logger, "content cleanup database", database)
}
