package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"esx/app/pipeline/behaviorlog/internal/config"
	behaviorlogic "esx/app/pipeline/behaviorlog/internal/logic"
	behaviorconsumer "esx/app/pipeline/behaviorlog/internal/mqs/behavior_log"
	"esx/app/pipeline/behaviorlog/internal/svc"
	"esx/pkg/cleanupx"
	"esx/pkg/mqx"

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

	go runDailyAggregation(c.AggregateIntervalSeconds, c.AggregateBackfillDays, svcCtx.Store, logger)

	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Behavior-log consumer started, subscribing: %s\n", c.MQ.Topic)
	<-shutdownCtx.Done()
}

// runDailyAggregation 周期执行 REL-020 去标识聚合：把最近 backfillDays 天的原始行为
// 聚合进 daily_aggregates（保留 365 天）。intervalSeconds <= 0 时禁用；
// 单次聚合带 5 分钟超时，失败只记录不中断消费。
func runDailyAggregation(intervalSeconds, backfillDays int, store svc.BehaviorStore, logger logx.Logger) {
	if intervalSeconds <= 0 || store == nil {
		return
	}
	runOnce := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		from, to := aggregateWindow(time.Now(), backfillDays)
		count, err := store.AggregateDaily(ctx, from, to)
		if err != nil {
			logx.WithContext(ctx).Errorw("REL-020 daily aggregate failed",
				logx.Field("err", err.Error()))
			return
		}
		logx.WithContext(ctx).Infow("REL-020 daily aggregate",
			logx.Field("from", from.Format(time.DateOnly)),
			logx.Field("to", to.Format(time.DateOnly)),
			logx.Field("rows", count))
	}
	// 启动后立即执行一次（配合 AggregateBackfillDays 回填存量），再进入周期。
	runOnce()
	interval := time.Duration(intervalSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		runOnce()
	}
}

// aggregateWindow 返回 [today-backfillDays, today) 的 UTC 日期窗口（默认含昨天）。
func aggregateWindow(now time.Time, backfillDays int) (time.Time, time.Time) {
	if backfillDays < 1 {
		backfillDays = 1
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return today.AddDate(0, 0, -backfillDays), today
}
