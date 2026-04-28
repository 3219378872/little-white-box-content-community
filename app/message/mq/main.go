package main

import (
	"context"
	"flag"
	"fmt"

	"cleanupx"
	"esx/app/message/mq/internal/config"
	"esx/app/message/mq/internal/mqs"
	"esx/app/message/mq/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

var configFile = flag.String("f", "etc/message-consumer.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	svcCtx := svc.NewServiceContext(c)

	msgConsumer, err := mqs.NewMessageConsumer(svcCtx)
	if err != nil {
		logx.Must(err)
	}
	if err := msgConsumer.Start(); err != nil {
		logx.Must(err)
	}
	defer cleanupx.Shutdown(logx.WithContext(context.Background()), "message notification consumer", msgConsumer.Shutdown)

	fmt.Println("Message MQ consumer started, subscribing message-push...")
	select {}
}
