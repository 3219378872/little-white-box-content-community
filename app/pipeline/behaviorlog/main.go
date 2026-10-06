package main

import (
	"context"
	"esx/pkg/lifecycle"
	"esx/pkg/rpcx"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"esx/app/pipeline/behaviorlog/internal/config"
	behaviorlogic "esx/app/pipeline/behaviorlog/internal/logic"
	behaviorconsumer "esx/app/pipeline/behaviorlog/internal/mqs/behavior_log"
	"esx/app/pipeline/behaviorlog/internal/svc"
	"esx/pkg/cleanupx"
	"esx/pkg/mqx"

	conf "esx/pkg/configx"
	"esx/pkg/logging"
)

var configFile = flag.String("f", "etc/behavior-log.yaml", "config file")

// main 启动行为日志消费者：把行为事件写入 ClickHouse，并在后台定时执行 REL-020 日聚合。
func main() {
	defer rpcx.CloseAllClients()
	defer lifecycle.CloseResources()
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	c.MustSetUp()

	svcCtx := svc.NewServiceContext(c)
	processor := behaviorlogic.NewRecorder(svcCtx.Store, svcCtx.Dedup)
	handler := behaviorconsumer.MakeBehaviorHandler(processor, svcCtx.DeadLetters)

	mq, err := mqx.NewConsumer(c.MQ)
	if err != nil {
		logging.Must(err)
	}

	if err := mq.Subscribe(handler); err != nil {
		logging.Must(fmt.Errorf("subscribe %s: %w", c.MQ.Topic, err))
	}

	if err := mq.Start(); err != nil {
		logging.Must(err)
	}
	logger := logging.WithContext(context.Background())

	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var background sync.WaitGroup
	background.Go(func() {
		runDailyAggregation(shutdownCtx, c.AggregateIntervalSeconds, c.AggregateBackfillDays, svcCtx.Store)
	})

	fmt.Printf("Behavior-log consumer started, subscribing: %s\n", c.MQ.Topic)
	<-shutdownCtx.Done()
	// 先停止消费并等待聚合退出，最后才关闭 ClickHouse 连接，避免在途写入失败。
	cleanupx.Shutdown(logger, "behavior-log consumer", mq.Shutdown)
	background.Wait()
	cleanupx.Shutdown(logger, "behavior-log clickhouse", svcCtx.Close)
}

// runDailyAggregation 周期执行 REL-020 去标识聚合：把最近 backfillDays 天的原始行为
// 聚合进 daily_aggregates（保留 365 天）。intervalSeconds <= 0 时禁用；
// 单次聚合带 5 分钟超时，失败只记录不中断消费。
func runDailyAggregation(ctx context.Context, intervalSeconds, backfillDays int, store svc.BehaviorStore) {
	if intervalSeconds <= 0 || store == nil {
		return
	}
	runOnce := func() bool {
		runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		from, to := aggregateWindow(time.Now(), backfillDays)
		count, err := store.AggregateDaily(runCtx, from, to)
		if err != nil {
			if ctx.Err() != nil {
				return false
			}
			logging.WithContext(runCtx).Errorw("REL-020 daily aggregate failed",
				logging.Field("err", err.Error()))
			return true
		}
		logging.WithContext(runCtx).Infow("REL-020 daily aggregate",
			logging.Field("from", from.Format(time.DateOnly)),
			logging.Field("to", to.Format(time.DateOnly)),
			logging.Field("rows", count))
		return true
	}
	// 启动后立即执行一次（配合 AggregateBackfillDays 回填存量），再进入周期。
	if ctx.Err() != nil || !runOnce() {
		return
	}
	interval := time.Duration(intervalSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !runOnce() {
				return
			}
		}
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
