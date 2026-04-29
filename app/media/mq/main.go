package main

import (
	"context"
	"flag"
	"fmt"

	"cleanupx"
	"esx/app/media/mq/internal/config"
	"esx/app/media/mq/internal/mqs"
	"esx/app/media/mq/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

var configFile = flag.String("f", "etc/media-consumer.yaml", "config file")

func main() {
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
	defer cleanupx.Shutdown(logx.WithContext(context.Background()), "media cleanup consumer", mqConsumer.Shutdown)

	fmt.Println("Media MQ consumer started, subscribing media-deleted...")
	select {}
}
