package main

import (
	"context"
	"flag"
	"fmt"

	"esx/app/search/mq/internal/config"
	"esx/app/search/mq/internal/mqs"
	"esx/app/search/mq/internal/svc"
	"esx/pkg/cleanupx"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/proc"
)

var configFile = flag.String("f", "etc/search-consumer.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	c.MustSetUp()

	svcCtx, err := svc.NewServiceContext(c)
	if err != nil {
		logx.Must(err)
	}

	searchConsumer, err := mqs.NewSearchConsumer(svcCtx)
	if err != nil {
		logx.Must(err)
	}
	if err := searchConsumer.Start(); err != nil {
		logx.Must(err)
	}
	defer cleanupx.Shutdown(logx.WithContext(context.Background()), "search consumer", searchConsumer.Shutdown)

	fmt.Println("Search MQ consumer started, subscribing post lifecycle topics...")
	<-proc.Done()
}
