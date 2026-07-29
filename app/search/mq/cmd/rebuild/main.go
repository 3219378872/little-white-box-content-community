package main

import (
	"context"
	"flag"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"esx/app/content/rpc/contentservice"
	"esx/app/search/mq/internal/config"
	"esx/app/search/mq/internal/indexer"
	"esx/app/search/mq/internal/rebuild"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/zrpc"
)

type commandConfig struct {
	ContentRpc zrpc.RpcClientConf
	ES         config.ESConfig
	Rebuild    struct {
		PageSize       int32
		TimeoutSeconds int64
	}
}

var configFile = flag.String("f", "app/search/mq/etc/search-consumer.yaml", "config file")

func main() {
	flag.Parse()
	var c commandConfig
	c.Rebuild.PageSize = 50
	c.Rebuild.TimeoutSeconds = 900
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	if c.Rebuild.PageSize <= 0 || c.Rebuild.PageSize > rebuild.MaxPageSize {
		panic(fmt.Sprintf("search rebuild page size must be between 1 and %d", rebuild.MaxPageSize))
	}
	if c.Rebuild.TimeoutSeconds <= 0 {
		panic("search rebuild timeout must be positive")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.Rebuild.TimeoutSeconds)*time.Second)
	defer cancel()

	opts := make([]indexer.ESOption, 0, 1)
	if c.ES.Username != "" {
		opts = append(opts, indexer.WithBasicAuth(c.ES.Username, c.ES.Password))
	}
	targetName := fmt.Sprintf("%s_rebuild_%d", c.ES.Index, time.Now().UTC().Unix())
	target, err := indexer.NewESIndexer(c.ES.Addresses, targetName, opts...)
	if err != nil {
		panic(err)
	}
	if err := target.EnsureIndex(ctx); err != nil {
		panic(err)
	}
	promoted := false
	defer func() {
		if !promoted {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanupCancel()
			_ = target.DeleteIndex(cleanupCtx)
		}
	}()

	contentClient := zrpc.MustNewClient(c.ContentRpc)
	source := contentservice.NewContentService(contentClient)
	count, err := rebuild.Run(ctx, source, target, c.Rebuild.PageSize)
	if err != nil {
		panic(err)
	}
	if err := target.PromoteToAlias(ctx, c.ES.Index); err != nil {
		panic(err)
	}
	promoted = true
	fmt.Printf("Search index rebuild completed: alias=%s index=%s posts=%d\n", c.ES.Index, targetName, count)
}
