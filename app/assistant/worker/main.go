package main

import (
	"context"
	"esx/pkg/rpcx"
	"flag"
	"fmt"
	"time"

	"esx/app/assistant/internal/runtime"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/worker/internal/config"
	"esx/app/assistant/worker/internal/svc"

	conf "esx/pkg/configx"
	proc "esx/pkg/lifecycle"
	"esx/pkg/logging"
)

var configFile = flag.String("f", "etc/agent.yaml", "config file")

func main() {
	defer rpcx.CloseAllClients()
	defer proc.CloseResources()
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	c.MustSetUp()

	svcCtx, err := svc.NewServiceContext(c)
	if err != nil {
		logging.Must(err)
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
			run, recovered, err := svcCtx.Lease.Claim(ctx)
			if err != nil {
				logging.WithContext(ctx).Errorw("assistant-agent claim failed", logging.Field("err", err.Error()))
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
				logging.WithContext(ctx).Errorw("assistant waiting scan failed", logging.Field("err", err.Error()))
				continue
			}
			for _, run := range runs {
				if err := runtime.ResolveWaiting(ctx, svcCtx.Store, nil, run.ID, store.NowMs()); err != nil {
					logging.WithContext(ctx).Errorw("assistant waiting resolution failed", logging.Field("runId", run.ID), logging.Field("err", err.Error()))
				}
			}
			if err := runtime.ExpireConfirmationWaits(ctx, svcCtx.Store, store.NowMs()); err != nil {
				logging.WithContext(ctx).Errorw("assistant confirmation wait scan failed", logging.Field("err", err.Error()))
			}
		}
	}
}

func runRetention(ctx context.Context, svcCtx *svc.ServiceContext) {
	run := func() {
		result, err := svcCtx.Retention.RunOnce(ctx)
		if err != nil {
			logging.WithContext(ctx).Errorw("assistant retention cleanup failed", logging.Field("err", err.Error()))
			return
		}
		if result.Messages > 0 {
			logging.WithContext(ctx).Infow("assistant retention cleanup completed",
				logging.Field("messages", result.Messages))
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
