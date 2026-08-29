package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"esx/app/assistant/internal/runtime"
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
	for {
		select {
		case <-ctx.Done():
			return
		case <-indexTicker.C:
			if svcCtx.Index != nil {
				_ = svcCtx.Index.Relay(ctx)
			}
		case <-ticker.C:
			runtime.ScheduleDueWatchRuns(ctx, svcCtx.Store, svcCtx.Watch)
			run, recovered, err := svcCtx.Lease.Claim(ctx)
			if err != nil {
				logx.WithContext(ctx).Errorw("assistant-agent claim failed", logx.Field("err", err.Error()))
				continue
			}
			if run == nil {
				continue
			}
			runCtx, runCancel := context.WithCancel(ctx)
			go svcCtx.Lease.RenewLoop(runCtx, run.ID)
			svcCtx.Engine.Execute(runCtx, *run, recovered)
			runCancel()
		}
	}
}
