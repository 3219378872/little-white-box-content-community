package main

import (
	"context"
	"flag"
	"fmt"

	"esx/app/assistant/mq/internal/config"
	"esx/app/assistant/mq/internal/mqs"
	"esx/app/assistant/mq/internal/svc"
	"esx/pkg/cleanupx"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/proc"
)

var configFile = flag.String("f", "etc/watch-consumer.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	c.MustSetUp()

	svcCtx, err := svc.NewServiceContext(c)
	if err != nil {
		logx.Must(err)
	}

	watchConsumer, err := mqs.NewWatchConsumer(svcCtx)
	if err != nil {
		logx.Must(err)
	}
	if err := watchConsumer.Start(); err != nil {
		logx.Must(err)
	}
	defer cleanupx.Shutdown(logx.WithContext(context.Background()), "watch matcher", watchConsumer.Shutdown)

	fmt.Println("Assistant watch matcher started, subscribing post lifecycle topics...")
	<-proc.Done()
}
