package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"cleanupx"
	"esx/app/recommend/mq/internal/config"
	"esx/app/recommend/mq/internal/mqs"
	"esx/app/recommend/mq/internal/store"
	"esx/app/recommend/mq/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
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
	defer cleanupx.Shutdown(logx.WithContext(context.Background()), "recommend consumer", recConsumer.Shutdown)

	go runOptOutCleanup(c.OptOutCleanupInterval, svcCtx.BehaviorStore)

	fmt.Println("Recommend MQ consumer started, subscribing user-behavior...")
	select {}
}

// runOptOutCleanup 周期执行 REL-023 主动清理：删除已关闭个性化用户的在线特征。
// intervalSeconds <= 0 时禁用；单次清理带 30s 超时，失败只记录不中断消费。
func runOptOutCleanup(intervalSeconds int, store store.BehaviorStore) {
	if intervalSeconds <= 0 {
		return
	}
	interval := time.Duration(intervalSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	baseCtx := context.Background()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(baseCtx, 30*time.Second)
		purged, err := store.PurgeOptedOutFeatures(ctx)
		if err != nil {
			logx.WithContext(ctx).Errorw("REL-023 opted-out feature cleanup failed",
				logx.Field("err", err.Error()))
		} else if purged > 0 {
			logx.WithContext(ctx).Infow("REL-023 opted-out feature cleanup",
				logx.Field("purged_users", purged))
		}
		cancel()
	}
}
