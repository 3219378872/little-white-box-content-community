package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"esx/app/ad/mq/internal/config"
	"esx/app/ad/mq/internal/mqs"
	"esx/app/ad/mq/internal/svc"
	"esx/pkg/cleanupx"
	"esx/pkg/rpcx"

	conf "esx/pkg/configx"
	proc "esx/pkg/lifecycle"
	"esx/pkg/logging"
)

var configFile = flag.String("f", "etc/ad-consumer.yaml", "config file")

// main 启动广告异步处理：消费审核结论更新投放状态，并在后台定期对账送审、重建投放索引与复扫。
func main() {
	defer rpcx.CloseAllClients()
	defer proc.CloseResources()
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	c.MustSetUp()
	if c.Assets.AccessKey == "" || c.Assets.SecretKey == "" {
		panic("ad-mq: S3_ACCESS_KEY and S3_SECRET_KEY must be set")
	}

	svcCtx, err := svc.NewServiceContext(c)
	if err != nil {
		logging.Must(err)
	}
	defer func() { _ = svcCtx.Close() }()

	decided, err := mqs.NewDecidedConsumer(svcCtx)
	if err != nil {
		logging.Must(err)
	}
	if err := decided.Start(); err != nil {
		logging.Must(err)
	}
	defer cleanupx.Shutdown(logging.WithContext(context.Background()), "ad decision consumer", decided.Shutdown)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	proc.AddShutdownListener(cancel)
	fmt.Println("Ad MQ consumer started, subscribing review-decided...")
	runTickers(ctx, svcCtx)
}

// runTickers：对账、资质到期与回扫间隔 1 分钟；索引按权威状态定期校正并重试素材发布。
func runTickers(ctx context.Context, svcCtx *svc.ServiceContext) {
	reconcile := time.NewTicker(time.Duration(max(svcCtx.Config.ReconcileIntervalMs, 5000)) * time.Millisecond)
	defer reconcile.Stop()
	rebuild := time.NewTicker(time.Duration(max(svcCtx.Config.RebuildIntervalMs, 10000)) * time.Millisecond)
	defer rebuild.Stop()
	svcCtx.Reconciler.RebuildIndex(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-reconcile.C:
			svcCtx.Reconciler.ReconcileSubmissions(ctx)
			svcCtx.Reconciler.ExpireQualifications(ctx)
			svcCtx.Rescanner.Run(ctx)
		case <-rebuild.C:
			svcCtx.Reconciler.RebuildIndex(ctx)
		}
	}
}
