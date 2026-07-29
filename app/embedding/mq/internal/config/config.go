package config

import (
	"fmt"
	"strings"

	"mqx"

	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	service.ServiceConf
	MQ               mqx.ConsumerConfig
	Embedding        EmbeddingConfig
	Milvus           MilvusConfig
	ContentRpc       zrpc.RpcClientConf `json:",optional"`
	Rebuild          RebuildConfig      `json:",optional"`
	StartupTimeoutMs int64              `json:",default=30000,range=[100:300000]"`
}

type EmbeddingConfig struct {
	Address       string
	ModelVersion  string
	Dim           int   `json:",range=[1:32768]"`
	TimeoutMs     int64 `json:",default=5000,range=[1:60000]"`
	MaxTextBytes  int   `json:",default=16384,range=[1:1048576]"`
	MaxBatchSize  int   `json:",default=64,range=[1:1024]"`
	MaxBatchBytes int   `json:",default=262144,range=[1:16777216]"`
}

type MilvusConfig struct {
	Address    string
	Collection string
	Dim        int    `json:",range=[1:32768]"`
	Username   string `json:",optional"`
	Password   string `json:",optional"`
	Database   string `json:",optional"`
}

type RebuildConfig struct {
	Alias            string
	CollectionPrefix string `json:",default=xbh_post_embeddings"`
	PageSize         int32  `json:",default=50,range=[1:50]"`
	BatchSize        int    `json:",default=32,range=[1:1024]"`
	MaxAttempts      int    `json:",default=3,range=[1:20]"`
	RetryBackoffMs   int64  `json:",default=500,range=[1:60000]"`
	TimeoutSeconds   int64  `json:",default=3600,range=[1:86400]"`
}

func (c Config) ValidateRebuild() error {
	if err := c.ValidateRuntime(); err != nil {
		return err
	}
	if strings.TrimSpace(c.Rebuild.Alias) == "" {
		return fmt.Errorf("Rebuild.Alias is required")
	}
	if c.Milvus.Collection != c.Rebuild.Alias {
		return fmt.Errorf("Milvus.Collection %q must equal Rebuild.Alias %q", c.Milvus.Collection, c.Rebuild.Alias)
	}
	if strings.TrimSpace(c.Rebuild.CollectionPrefix) == "" {
		return fmt.Errorf("Rebuild.CollectionPrefix is required")
	}
	if c.Rebuild.PageSize <= 0 || c.Rebuild.PageSize > 50 {
		return fmt.Errorf("Rebuild.PageSize must be between 1 and 50")
	}
	if c.Rebuild.BatchSize <= 0 || c.Rebuild.BatchSize > c.Embedding.MaxBatchSize {
		return fmt.Errorf("Rebuild.BatchSize must be between 1 and Embedding.MaxBatchSize")
	}
	if c.Rebuild.MaxAttempts <= 0 || c.Rebuild.RetryBackoffMs <= 0 || c.Rebuild.TimeoutSeconds <= 0 {
		return fmt.Errorf("rebuild retry and timeout values must be positive")
	}
	if len(c.ContentRpc.Endpoints) == 0 && strings.TrimSpace(c.ContentRpc.Target) == "" && strings.TrimSpace(c.ContentRpc.Etcd.Key) == "" {
		return fmt.Errorf("ContentRpc endpoint, target, or etcd key is required")
	}
	return nil
}

func (c Config) ValidateRuntime() error {
	if strings.TrimSpace(c.Embedding.Address) == "" {
		return fmt.Errorf("Embedding.Address is required")
	}
	if strings.TrimSpace(c.Embedding.ModelVersion) == "" {
		return fmt.Errorf("Embedding.ModelVersion is required")
	}
	if len(c.Embedding.ModelVersion) > 256 {
		return fmt.Errorf("Embedding.ModelVersion must not exceed 256 bytes")
	}
	if c.Embedding.Dim <= 0 {
		return fmt.Errorf("Embedding.Dim must be positive")
	}
	if c.Embedding.TimeoutMs <= 0 || c.Embedding.MaxTextBytes <= 0 || c.Embedding.MaxBatchSize <= 0 || c.Embedding.MaxBatchBytes <= 0 {
		return fmt.Errorf("embedding timeout and input limits must be positive")
	}
	if c.Embedding.MaxBatchBytes < c.Embedding.MaxTextBytes {
		return fmt.Errorf("Embedding.MaxBatchBytes must be at least Embedding.MaxTextBytes")
	}
	if strings.TrimSpace(c.Milvus.Address) == "" {
		return fmt.Errorf("Milvus.Address is required")
	}
	if strings.TrimSpace(c.Milvus.Collection) == "" {
		return fmt.Errorf("Milvus.Collection is required")
	}
	if c.Milvus.Dim != c.Embedding.Dim {
		return fmt.Errorf("Milvus.Dim %d must equal Embedding.Dim %d", c.Milvus.Dim, c.Embedding.Dim)
	}
	if c.StartupTimeoutMs <= 0 {
		return fmt.Errorf("StartupTimeoutMs must be positive")
	}
	return nil
}
