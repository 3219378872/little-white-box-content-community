package main

import (
	"context"
	"flag"
	"fmt"

	"cleanupx"
	"esx/app/search/mq/internal/config"
	"esx/app/search/mq/internal/mqs"
	"esx/app/search/mq/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

var configFile = flag.String("f", "etc/search-consumer.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	svcCtx := svc.NewServiceContext(c)

	searchConsumer, err := mqs.NewSearchConsumer(svcCtx)
	if err != nil {
		logx.Must(err)
	}
	if err := searchConsumer.Start(); err != nil {
		logx.Must(err)
	}
	defer cleanupx.Shutdown(logx.WithContext(context.Background()), "search consumer", searchConsumer.Shutdown)

	fmt.Println("Search MQ consumer started, subscribing search-index...")
	select {}
}
