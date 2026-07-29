package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/zrpc"
)

func validConfig() Config {
	return Config{
		Embedding: EmbeddingConfig{
			Address: "127.0.0.1:50051", ModelVersion: "model@sha", Dim: 384,
			TimeoutMs: 1000, MaxTextBytes: 1024, MaxBatchSize: 16, MaxBatchBytes: 4096,
		},
		Milvus:     MilvusConfig{Address: "127.0.0.1:19530", Collection: "post_embeddings_current", Dim: 384},
		ContentRpc: zrpc.RpcClientConf{Target: "dns:///content:8088"},
		Rebuild: RebuildConfig{
			Alias: "post_embeddings_current", CollectionPrefix: "post_embeddings",
			PageSize: 50, BatchSize: 16, MaxAttempts: 3, RetryBackoffMs: 10, TimeoutSeconds: 60,
		},
		StartupTimeoutMs: 1000,
	}
}

func TestValidateRebuildRequiresStableAliasAndBoundedBatch(t *testing.T) {
	c := validConfig()
	require.NoError(t, c.ValidateRebuild())

	c.Rebuild.Alias = "other"
	require.ErrorContains(t, c.ValidateRebuild(), "must equal")
	c = validConfig()
	c.Rebuild.BatchSize = c.Embedding.MaxBatchSize + 1
	require.ErrorContains(t, c.ValidateRebuild(), "Embedding.MaxBatchSize")
}

func TestValidateRuntimeRejectsMissingDependenciesAndDimensionDrift(t *testing.T) {
	c := validConfig()
	require.NoError(t, c.ValidateRuntime())

	c.Embedding.Address = ""
	require.ErrorContains(t, c.ValidateRuntime(), "Embedding.Address")
	c = validConfig()
	c.Milvus.Address = ""
	require.ErrorContains(t, c.ValidateRuntime(), "Milvus.Address")
	c = validConfig()
	c.Milvus.Dim = 256
	require.ErrorContains(t, c.ValidateRuntime(), "must equal")
}
