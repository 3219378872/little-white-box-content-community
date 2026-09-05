package main

import (
	"context"
	"flag"
	"fmt"
	"sync"
	"time"

	"esx/app/recommend/mq/internal/config"
	"esx/app/recommend/mq/internal/mqs"
	"esx/app/recommend/mq/internal/store"
	"esx/app/recommend/mq/internal/svc"
	"esx/pkg/cleanupx"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/proc"
)

var configFile = flag.String("f", "etc/recommend-consumer.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	c.MustSetUp()

	svcCtx := svc.NewServiceContext(c)

	recConsumer, err := mqs.NewRecommendConsumer(svcCtx)
	if err != nil {
		logx.Must(err)
	}
	if err := recConsumer.Start(); err != nil {
		logx.Must(err)
	}

	cleanupCtx, cancelCleanup := context.WithCancel(context.Background())
	var background sync.WaitGroup
	background.Go(func() {
		runOptOutCleanup(cleanupCtx, c.OptOutCleanupInterval, svcCtx.BehaviorStore)
	})

	fmt.Println("Recommend MQ consumer started, subscribing user-behavior...")
	<-proc.Done()
	cancelCleanup()
	cleanupx.Shutdown(logx.WithContext(context.Background()), "recommend consumer", recConsumer.Shutdown)
	background.Wait()
}

// runOptOutCleanup 周期执行 REL-023 主动清理：删除已关闭个性化用户的在线特征。
// intervalSeconds <= 0 时禁用；单次清理带 30s 超时，失败只记录不中断消费。
func runOptOutCleanup(ctx context.Context, intervalSeconds int, store store.BehaviorStore) {
	if intervalSeconds <= 0 {
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
		}
		purgeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		purged, err := store.PurgeOptedOutFeatures(purgeCtx)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logx.WithContext(ctx).Errorw("REL-023 opted-out feature cleanup failed",
				logx.Field("err", err.Error()))
		} else if purged > 0 {
			logx.WithContext(ctx).Infow("REL-023 opted-out feature cleanup",
				logx.Field("purged_users", purged))
		}
	}
}
