package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"cleanupx"
	"esx/app/pipeline/behaviorlog/internal/config"
	behaviorlogic "esx/app/pipeline/behaviorlog/internal/logic"
	behaviorconsumer "esx/app/pipeline/behaviorlog/internal/mqs/behavior_log"
	"esx/app/pipeline/behaviorlog/internal/svc"
	"mqx"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

var configFile = flag.String("f", "etc/behavior-log.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	c.MustSetUp()

	svcCtx := svc.NewServiceContext(c)
	processor := behaviorlogic.NewRecorder(svcCtx.Store, svcCtx.Dedup)
	handler := behaviorconsumer.MakeBehaviorHandler(processor, svcCtx.DeadLetters)

	mq, err := mqx.NewConsumer(c.MQ)
	if err != nil {
		logx.Must(err)
	}

	if err := mq.Subscribe(handler); err != nil {
		logx.Must(fmt.Errorf("subscribe %s: %w", c.MQ.Topic, err))
	}

	if err := mq.Start(); err != nil {
		logx.Must(err)
	}
	logger := logx.WithContext(context.Background())
	defer cleanupx.Shutdown(logger, "behavior-log clickhouse", svcCtx.Close)
	defer cleanupx.Shutdown(logger, "behavior-log consumer", mq.Shutdown)

	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Behavior-log consumer started, subscribing: %s\n", c.MQ.Topic)
	<-shutdownCtx.Done()
}
