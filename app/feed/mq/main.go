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
	"esx/pkg/logging"
)

var configFile = flag.String("f", "etc/feed-consumer.yaml", "config file")

// main 启动关注流 fanout 消费者，进程退出前优雅停止消费。
func main() {
	defer rpcx.CloseAllClients()
	defer proc.CloseResources()
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	svcCtx := svc.NewServiceContext(c)

	postConsumer, err := mqs.NewPostPublishConsumer(svcCtx)
	if err != nil {
		logging.Must(err)
	}
	if err := postConsumer.Start(); err != nil {
		logging.Must(err)
	}
	defer cleanupx.Shutdown(logging.WithContext(context.Background()), "feed post-publish consumer", postConsumer.Shutdown)

	fmt.Println("Feed MQ consumer started, subscribing post-create...")
	<-proc.Done()
}
