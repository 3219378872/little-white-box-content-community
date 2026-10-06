package main

import (
	"context"
	"esx/pkg/rpcx"
	"flag"
	"fmt"

	"esx/app/search/mq/internal/config"
	"esx/app/search/mq/internal/mqs"
	"esx/app/search/mq/internal/svc"
	"esx/pkg/cleanupx"

	conf "esx/pkg/configx"
	proc "esx/pkg/lifecycle"
	"esx/pkg/logging"
)

var configFile = flag.String("f", "etc/search-consumer.yaml", "config file")

// main 启动帖子生命周期事件的搜索索引消费者，进程退出前优雅停止消费。
func main() {
	defer rpcx.CloseAllClients()
	defer proc.CloseResources()
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	c.MustSetUp()

	// 索引不可用时直接终止启动，避免确认消息却没有写入索引。
	svcCtx, err := svc.NewServiceContext(c)
	if err != nil {
		logging.Must(err)
	}

	searchConsumer, err := mqs.NewSearchConsumer(svcCtx)
	if err != nil {
		logging.Must(err)
	}
	if err := searchConsumer.Start(); err != nil {
		logging.Must(err)
	}
	defer cleanupx.Shutdown(logging.WithContext(context.Background()), "search consumer", searchConsumer.Shutdown)

	fmt.Println("Search MQ consumer started, subscribing post lifecycle topics...")
	<-proc.Done()
}
