package main

import (
	"context"
	"esx/pkg/rpcx"
	"flag"
	"fmt"

	"esx/app/content/mq/cleanup/internal/config"
	"esx/app/content/mq/cleanup/internal/mqs"
	"esx/app/content/mq/cleanup/internal/svc"
	"esx/pkg/cleanupx"

	conf "esx/pkg/configx"
	proc "esx/pkg/lifecycle"
	"esx/pkg/logging"
)

var configFile = flag.String("f", "etc/content-cleanup.yaml", "config file")

func main() {
	defer rpcx.CloseAllClients()
	defer proc.CloseResources()
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	svcCtx := svc.NewServiceContext(c)

	cleanupConsumer, err := mqs.NewCleanupConsumer(svcCtx)
	if err != nil {
		logging.Must(err)
	}
	if err := cleanupConsumer.Start(); err != nil {
		logging.Must(err)
	}
	logger := logging.WithContext(context.Background())
	var countSyncShutdown func() error
	defer func() {
		shutdownContentCleanup(logger, countSyncShutdown, cleanupConsumer.Shutdown, svcCtx.Close)
	}()

	if svcCtx.CountSyncStore != nil {
		countSyncConsumer, err := mqs.NewCountSyncConsumer(svcCtx)
		if err != nil {
			logging.Must(err)
		}
		if err := countSyncConsumer.Start(); err != nil {
			logging.Must(err)
		}
		countSyncShutdown = countSyncConsumer.Shutdown
	}

	fmt.Println("Content cleanup MQ consumer started, subscribing post-delete and behavior counts...")
	<-proc.Done()
}

func shutdownContentCleanup(logger logging.Logger, countSync, cleanup, database func() error) {
	cleanupx.Shutdown(logger, "count-sync consumer", countSync)
	cleanupx.Shutdown(logger, "content-cleanup consumer", cleanup)
	cleanupx.Shutdown(logger, "content cleanup database", database)
}
