package main

import (
	"context"
	"esx/pkg/lifecycle"
	"flag"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"esx/app/content/rpc/contentservice"
	"esx/app/search/mq/internal/config"
	"esx/app/search/mq/internal/indexer"
	"esx/app/search/mq/internal/rebuild"

	conf "esx/pkg/configx"
	"esx/pkg/rpcx"
)

// commandConfig 是离线重建命令的配置，复用消费者配置文件中的 ES 与内容服务段。
type commandConfig struct {
	ContentRpc     rpcx.RpcClientConf
	InternalSecret string
	ES             config.ESConfig
	Rebuild        struct {
		PageSize       int32
		TimeoutSeconds int64
	}
}

var configFile = flag.String("f", "app/search/mq/etc/search-consumer.yaml", "config file")

// main 离线重建搜索索引：写入带时间戳的新索引，成功后把别名切换过去；
// 任何失败都删除新索引，线上别名保持原状。
func main() {
	defer rpcx.CloseAllClients()
	defer lifecycle.CloseResources()
	flag.Parse()
	var c commandConfig
	// 默认值先于配置加载设置，配置文件未声明时生效。
	c.Rebuild.PageSize = 50
	c.Rebuild.TimeoutSeconds = 900
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	if c.Rebuild.PageSize <= 0 || c.Rebuild.PageSize > rebuild.MaxPageSize {
		panic(fmt.Sprintf("search rebuild page size must be between 1 and %d", rebuild.MaxPageSize))
	}
	if c.Rebuild.TimeoutSeconds <= 0 {
		panic("search rebuild timeout must be positive")
	}

	// 支持 Ctrl-C 中断，并为整次重建设总超时。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.Rebuild.TimeoutSeconds)*time.Second)
	defer cancel()

	opts := make([]indexer.ESOption, 0, 1)
	if c.ES.Username != "" {
		opts = append(opts, indexer.WithBasicAuth(c.ES.Username, c.ES.Password))
	}
	// 新索引名带时间戳，与线上别名指向的旧索引互不干扰。
	targetName := fmt.Sprintf("%s_rebuild_%d", c.ES.Index, time.Now().UTC().Unix())
	target, err := indexer.NewESIndexer(c.ES.Addresses, targetName, opts...)
	if err != nil {
		panic(err)
	}
	if err := target.EnsureIndex(ctx); err != nil {
		panic(err)
	}
	// 未成功切换别名时清理半成品索引；用独立上下文，避免已取消的 ctx 让清理失效。
	promoted := false
	defer func() {
		if !promoted {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanupCancel()
			_ = target.DeleteIndex(cleanupCtx)
		}
	}()

	contentClient := rpcx.MustNewClient(c.ContentRpc,
		rpcx.WithInternalAuth(c.InternalSecret))
	source := contentservice.NewContentService(contentClient)
	count, err := rebuild.Run(ctx, source, target, c.Rebuild.PageSize)
	if err != nil {
		panic(err)
	}
	// 全部写入成功后才原子切换别名，读路径无感知。
	if err := target.PromoteToAlias(ctx, c.ES.Index); err != nil {
		panic(err)
	}
	promoted = true
	fmt.Printf("Search index rebuild completed: alias=%s index=%s posts=%d\n", c.ES.Index, targetName, count)
}
