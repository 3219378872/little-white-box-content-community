package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"esx/app/assistant/internal/runtime"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/worker/internal/config"
	"esx/app/assistant/worker/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/proc"
)

var configFile = flag.String("f", "etc/agent.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	c.MustSetUp()

	svcCtx, err := svc.NewServiceContext(c)
	if err != nil {
		logx.Must(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	proc.AddShutdownListener(cancel)

	fmt.Println("Assistant agent worker started")
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	indexTicker := time.NewTicker(2 * time.Second)
	defer indexTicker.Stop()
	go runRetention(ctx, svcCtx)
	go runWaitingExpiry(ctx, svcCtx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-indexTicker.C:
			if svcCtx.Index != nil {
				_ = svcCtx.Index.Relay(ctx)
			}
		case <-ticker.C:
			runtime.ScheduleDueWatchRuns(ctx, svcCtx.Store, svcCtx.Memory, svcCtx.Watch, svcCtx.Consent, svcCtx.WatchPosts)
			run, recovered, err := svcCtx.Lease.Claim(ctx)
			if err != nil {
				logx.WithContext(ctx).Errorw("assistant-agent claim failed", logx.Field("err", err.Error()))
				continue
			}
			if run == nil {
				continue
			}
			runCtx, runCancel := context.WithCancel(ctx)
			go svcCtx.Lease.RenewLoop(runCtx, *run, runCancel)
			svcCtx.Engine.Execute(runCtx, *run, recovered)
			runCancel()
		}
	}
}

func runWaitingExpiry(ctx context.Context, svcCtx *svc.ServiceContext) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runs, err := svcCtx.Store.ListWaitingRuns(ctx)
			if err != nil {
				logx.WithContext(ctx).Errorw("assistant waiting scan failed", logx.Field("err", err.Error()))
				continue
			}
			for _, run := range runs {
				if err := runtime.ResolveWaiting(ctx, svcCtx.Store, nil, run.ID, store.NowMs()); err != nil {
					logx.WithContext(ctx).Errorw("assistant waiting resolution failed", logx.Field("runId", run.ID), logx.Field("err", err.Error()))
				}
			}
		}
	}
}

func runRetention(ctx context.Context, svcCtx *svc.ServiceContext) {
	run := func() {
		result, err := svcCtx.Retention.RunOnce(ctx)
		if err != nil {
			logx.WithContext(ctx).Errorw("assistant retention cleanup failed", logx.Field("err", err.Error()))
			return
		}
		if result.Messages+result.WatchHits+result.WatchExecutions > 0 {
			logx.WithContext(ctx).Infow("assistant retention cleanup completed",
				logx.Field("messages", result.Messages),
				logx.Field("watch_hits", result.WatchHits),
				logx.Field("watch_executions", result.WatchExecutions))
		}
	}
	run()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
