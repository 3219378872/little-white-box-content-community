package main

import (
	"context"
	"flag"
	"fmt"

	"cleanupx"
	"esx/app/embedding/mq/internal/config"
	"esx/app/embedding/mq/internal/mqs"
	"esx/app/embedding/mq/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

var configFile = flag.String("f", "etc/embedding-consumer.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	svcCtx := svc.NewServiceContext(c)

	embeddingConsumer, err := mqs.NewEmbeddingConsumer(svcCtx)
	if err != nil {
		logx.Must(err)
	}
	if err := embeddingConsumer.Start(); err != nil {
		logx.Must(err)
	}
	defer cleanupx.Shutdown(logx.WithContext(context.Background()), "embedding consumer", embeddingConsumer.Shutdown)

	fmt.Println("Embedding MQ consumer started, subscribing post-create/update/delete...")
	select {}
}
