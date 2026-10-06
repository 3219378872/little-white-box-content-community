package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"esx/app/review/internal/metrics"
	"esx/app/review/internal/router"
	"esx/app/review/internal/store"
	"esx/app/review/worker/internal/config"
	"esx/app/review/worker/internal/mqs"
	"esx/app/review/worker/internal/svc"
	"esx/pkg/cleanupx"
	"esx/pkg/outboxx"
	"esx/pkg/rpcx"

	conf "esx/pkg/configx"
	proc "esx/pkg/lifecycle"
	"esx/pkg/logging"
)

var configFile = flag.String("f", "etc/review-worker.yaml", "config file")

// main 启动审核 worker：接入送审消息、轮询机审队列、同步种子库并上报积压指标。
func main() {
	defer rpcx.CloseAllClients()
	defer proc.CloseResources()
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	c.MustSetUp()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	proc.AddShutdownListener(cancel)

	svcCtx, err := svc.NewServiceContext(ctx, c)
	if err != nil {
		logging.Must(err)
	}
	defer func() {
		if err := svcCtx.Close(); err != nil {
			logging.Errorw("close review worker dependencies", logging.Field("err", err.Error()))
		}
	}()
	activatePolicies(ctx, svcCtx)

	if svcCtx.OutboxRelay != nil {
		relay := outboxx.StartRelay(ctx, svcCtx.OutboxRelay)
		defer func() {
			if err := relay.Stop(); err != nil {
				logging.Errorw("review worker outbox relay stopped", logging.Field("err", err.Error()))
			}
		}()
	}
	submitted, err := mqs.NewSubmittedConsumer(svcCtx)
	if err != nil {
		logging.Must(err)
	}
	if err := submitted.Start(); err != nil {
		logging.Must(err)
	}
	defer cleanupx.Shutdown(logging.WithContext(context.Background()), "review submitted consumer", submitted.Shutdown)

	fmt.Println("Review worker started")
	runLoops(ctx, svcCtx)
}

// activatePolicies 记录本进程激活的政策版本与配置哈希（RVW-025）。
func activatePolicies(ctx context.Context, svcCtx *svc.ServiceContext) {
	entries := []store.AuditEntry{{Action: "policy.activate", ObjectType: "policy",
		After: map[string]any{"version": svcCtx.Active.Version, "configHash": svcCtx.Active.ConfigHash, "mode": "active"}}}
	if svcCtx.Shadow != nil {
		entries = append(entries, store.AuditEntry{Action: "policy.activate", ObjectType: "policy",
			After: map[string]any{"version": svcCtx.Shadow.Version, "configHash": svcCtx.Shadow.ConfigHash, "mode": "shadow"}})
	}
	for _, entry := range entries {
		if err := svcCtx.Store.Audit(ctx, entry, time.Now()); err != nil {
			logging.Errorw("record policy activation failed", logging.Field("err", err.Error()))
		}
	}
}

// runLoops 按各自周期驱动机审队列、种子同步与积压指标，直到进程退出。
func runLoops(ctx context.Context, svcCtx *svc.ServiceContext) {
	poll := time.NewTicker(time.Duration(max(svcCtx.Config.PollIntervalMs, 100)) * time.Millisecond)
	defer poll.Stop()
	seeds := time.NewTicker(30 * time.Second)
	defer seeds.Stop()
	backlog := time.NewTicker(15 * time.Second)
	defer backlog.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
			drainMachineQueue(ctx, svcCtx)
		case <-seeds.C:
			syncSeeds(ctx, svcCtx)
		case <-backlog.C:
			if b, err := svcCtx.Store.HumanBacklog(ctx); err == nil {
				metrics.Backlog(b.Pending, b.OldestSubmit, time.Now())
			}
		}
	}
}

// drainMachineQueue 每轮最多处理 20 个任务，避免长时间占用循环。
func drainMachineQueue(ctx context.Context, svcCtx *svc.ServiceContext) {
	for range 20 {
		task, err := svcCtx.Store.ClaimMachine(ctx, time.Now())
		if err != nil {
			logging.WithContext(ctx).Errorw("review worker claim failed", logging.Field("err", err.Error()))
			return
		}
		if task == nil {
			return
		}
		if err := svcCtx.Processor.Process(ctx, task); err != nil {
			// 租约到期后任务会被重领；阶段记录按（task, stage, version）幂等。
			logging.WithContext(ctx).Errorw("review worker process failed",
				logging.Field("taskId", task.ID), logging.Field("err", err.Error()))
			return
		}
	}
}

// syncSeeds 把已生效的种子同步到向量索引；未配置向量依赖时跳过。
func syncSeeds(ctx context.Context, svcCtx *svc.ServiceContext) {
	if svcCtx.Embedder == nil || svcCtx.SeedIndex == nil {
		return
	}
	if _, err := router.SyncSeeds(ctx, svcCtx.Store, svcCtx.Embedder, svcCtx.SeedIndex, 50); err != nil {
		logging.WithContext(ctx).Errorw("review seed sync failed", logging.Field("err", err.Error()))
	}
}
