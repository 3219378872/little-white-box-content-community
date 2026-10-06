package main

import (
	"context"
	"esx/pkg/rpcx"
	"flag"
	"fmt"

	"esx/app/media/mq/internal/config"
	"esx/app/media/mq/internal/mqs"
	"esx/app/media/mq/internal/svc"
	"esx/pkg/cleanupx"

	conf "esx/pkg/configx"
	proc "esx/pkg/lifecycle"
	"esx/pkg/logging"
)

var configFile = flag.String("f", "etc/media-consumer.yaml", "config file")

// main 启动媒体对象清理消费者，删除已软删媒体与上传补偿留下的 S3 对象。
func main() {
	defer rpcx.CloseAllClients()
	defer proc.CloseResources()
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	if c.S3Storage.AccessKey == "" || c.S3Storage.SecretKey == "" {
		panic("media-consumer: S3_ACCESS_KEY and S3_SECRET_KEY must be set")
	}

	svcCtx := svc.NewServiceContext(c)

	mqConsumer, err := mqs.NewMediaCleanupConsumer(svcCtx)
	if err != nil {
		panic(fmt.Sprintf("media-consumer: MQ consumer init failed: %v", err))
	}
	if err := mqConsumer.Start(); err != nil {
		panic(fmt.Sprintf("media-consumer: MQ consumer start failed: %v", err))
	}
	defer cleanupx.Shutdown(logging.WithContext(context.Background()), "media cleanup consumer", mqConsumer.Shutdown)

	fmt.Println("Media MQ consumer started, subscribing media-deleted...")
	<-proc.Done()
}
