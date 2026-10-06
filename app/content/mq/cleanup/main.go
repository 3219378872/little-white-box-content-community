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

// main 启动帖子删除清理消费者，配置了计数同步时一并启动计数同步消费者。
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

// shutdownContentCleanup 先停两个消费者，最后关闭数据库，避免在途消息写入已关闭的连接。
func shutdownContentCleanup(logger logging.Logger, countSync, cleanup, database func() error) {
	cleanupx.Shutdown(logger, "count-sync consumer", countSync)
	cleanupx.Shutdown(logger, "content-cleanup consumer", cleanup)
	cleanupx.Shutdown(logger, "content cleanup database", database)
}
