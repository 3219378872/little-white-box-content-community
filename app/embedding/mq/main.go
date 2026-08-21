package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"esx/app/embedding/mq/internal/config"
	"esx/app/embedding/mq/internal/mqs"
	"esx/app/embedding/mq/internal/svc"
	"esx/pkg/cleanupx"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

var configFile = flag.String("f", "etc/embedding-consumer.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	c.MustSetUp()

	startupCtx, startupCancel := context.WithTimeout(context.Background(), time.Duration(c.StartupTimeoutMs)*time.Millisecond)
	defer startupCancel()
	svcCtx, err := svc.NewServiceContext(startupCtx, c)
	if err != nil {
		logx.Must(err)
	}
	logger := logx.WithContext(context.Background())
	defer cleanupx.Shutdown(logger, "embedding dependencies", svcCtx.Close)

	embeddingConsumer, err := mqs.NewEmbeddingConsumer(svcCtx)
	if err != nil {
		logx.Must(err)
	}
	if err := embeddingConsumer.Start(); err != nil {
		logx.Must(err)
	}
	defer cleanupx.Shutdown(logger, "embedding consumer", embeddingConsumer.Shutdown)

	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Println("Embedding MQ consumer started, subscribing post-create/update/delete...")
	<-shutdownCtx.Done()
}
