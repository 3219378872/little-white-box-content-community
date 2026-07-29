package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"esx/app/content/rpc/contentservice"
	"esx/app/embedding/mq/internal/config"
	"esx/app/embedding/mq/internal/embedder"
	"esx/app/embedding/mq/internal/rebuild"
	"esx/app/embedding/mq/internal/vectorstore"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

var (
	configFile = flag.String("f", "app/embedding/mq/etc/embedding-consumer.yaml", "config file")
	targetName = flag.String("target", "", "versioned target collection; generated when empty")
)

func main() {
	flag.Parse()
	if err := run(); err != nil {
		logx.Must(err)
	}
}

func run() (err error) {
	var c config.Config
	if err := conf.Load(*configFile, &c, conf.UseEnv()); err != nil {
		return fmt.Errorf("load embedding rebuild config: %w", err)
	}
	if err := c.ValidateRebuild(); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.Rebuild.TimeoutSeconds)*time.Second)
	defer cancel()

	emb, err := embedder.NewGRPCEmbedder(ctx, embedder.ClientConfig{
		Address:              c.Embedding.Address,
		ExpectedModelVersion: c.Embedding.ModelVersion,
		ExpectedDimension:    c.Embedding.Dim,
		Timeout:              time.Duration(c.Embedding.TimeoutMs) * time.Millisecond,
		MaxTextBytes:         c.Embedding.MaxTextBytes,
		MaxBatchSize:         c.Embedding.MaxBatchSize,
		MaxBatchBytes:        c.Embedding.MaxBatchBytes,
	})
	if err != nil {
		return fmt.Errorf("initialize embedding client: %w", err)
	}
	defer func() {
		if closeErr := emb.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close embedding client: %w", closeErr)
		}
	}()

	physicalTarget := *targetName
	if physicalTarget == "" {
		physicalTarget, err = rebuild.VersionedCollectionName(c.Rebuild.CollectionPrefix, c.Embedding.ModelVersion, time.Now())
		if err != nil {
			return err
		}
	}
	if physicalTarget == c.Rebuild.Alias {
		return fmt.Errorf("rebuild target %q must differ from the active alias", physicalTarget)
	}
	opts := make([]vectorstore.MilvusOption, 0, 2)
	if c.Milvus.Username != "" {
		opts = append(opts, vectorstore.WithMilvusAuth(c.Milvus.Username, c.Milvus.Password))
	}
	if c.Milvus.Database != "" {
		opts = append(opts, vectorstore.WithMilvusDatabase(c.Milvus.Database))
	}
	target, err := vectorstore.NewMilvusVectorStore(ctx, c.Milvus.Address, physicalTarget, c.Milvus.Dim, opts...)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := target.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close Milvus rebuild target: %w", closeErr)
		}
	}()
	if err := target.CreateCollection(ctx); err != nil {
		return err
	}
	promoted := false
	defer func() {
		if promoted {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := target.Drop(cleanupCtx); err != nil {
			logx.WithContext(cleanupCtx).Errorf("drop failed embedding rebuild target %s: %v", physicalTarget, err)
		}
	}()

	contentClient, err := zrpc.NewClient(c.ContentRpc)
	if err != nil {
		return fmt.Errorf("initialize Content RPC client: %w", err)
	}
	contentConn := contentClient.Conn()
	defer func() {
		if closeErr := contentConn.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close Content RPC client: %w", closeErr)
		}
	}()
	count, err := rebuild.RunAndPromote(
		ctx,
		contentservice.NewContentService(contentClient),
		emb,
		target,
		c.Rebuild.Alias,
		rebuild.Options{
			PageSize: c.Rebuild.PageSize, BatchSize: c.Rebuild.BatchSize,
			MaxAttempts:  c.Rebuild.MaxAttempts,
			RetryBackoff: time.Duration(c.Rebuild.RetryBackoffMs) * time.Millisecond,
		},
	)
	if err != nil {
		return err
	}
	promoted = true
	fmt.Printf("Embedding rebuild completed: alias=%s collection=%s posts=%d model=%s dimension=%d\n",
		c.Rebuild.Alias, physicalTarget, count, c.Embedding.ModelVersion, c.Embedding.Dim)
	return nil
}
