package vectorstore

import (
	"context"
	"esx/pkg/milvusx"
	"fmt"
	"strings"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
)

const (
	modelVersionMaxLength       = 256
	defaultMilvusConnectTimeout = 90 * time.Second
	milvusConnectRetryInterval  = 500 * time.Millisecond
)

// MilvusVectorStore 把帖子向量按 (post_id, revision) 写入 Milvus 集合，供搜索与推荐召回。
type MilvusVectorStore struct {
	cli        client.Client
	collection string
	dim        int
}

// MilvusOption 配置连接参数。
type MilvusOption func(*milvusOptions)

// milvusOptions 是可选的认证与数据库参数。
type milvusOptions struct {
	username string
	password string
	dbName   string
}

// WithMilvusAuth 设置用户名与密码。
func WithMilvusAuth(user, password string) MilvusOption {
	return func(o *milvusOptions) {
		o.username = user
		o.password = password
	}
}

// WithMilvusDatabase 指定数据库；为空时使用默认库。
func WithMilvusDatabase(db string) MilvusOption {
	return func(o *milvusOptions) {
		o.dbName = db
	}
}

// NewMilvusVectorStore 连接 Milvus 并等待其就绪；集合由 OpenCollection / EnsureCollection 另行打开。
func NewMilvusVectorStore(ctx context.Context, addr, collection string, dim int, opts ...MilvusOption) (*MilvusVectorStore, error) {
	if strings.TrimSpace(addr) == "" {
		return nil, fmt.Errorf("milvus address is required")
	}
	if strings.TrimSpace(collection) == "" {
		return nil, fmt.Errorf("milvus collection is required")
	}
	if dim <= 0 {
		return nil, fmt.Errorf("milvus vector dimension must be positive")
	}
	o := &milvusOptions{}
	for _, opt := range opts {
		opt(o)
	}
	connectCtx := ctx
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok {
		connectCtx, cancel = context.WithTimeout(ctx, defaultMilvusConnectTimeout)
	}
	defer cancel()

	cli, err := milvusx.Dial(connectCtx, client.Config{
		Address: addr, Username: o.username, Password: o.password, DBName: o.dbName,
	}, milvusConnectRetryInterval)
	if err != nil {
		return nil, fmt.Errorf("milvus connect: %w", err)
	}
	return &MilvusVectorStore{cli: cli, collection: collection, dim: dim}, nil
}

// waitReady 在最多 90 秒（或 ctx 更早的截止时间）内每 2 秒探测一次，只容忍 Milvus 启动期错误。
func (m *MilvusVectorStore) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(90 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	for {
		_, err := m.cli.HasCollection(ctx, "__embedding_readiness_probe__")
		if err == nil {
			return nil
		}
		if !milvusx.IsStartupError(err) {
			return fmt.Errorf("milvus readiness probe: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("milvus not ready before deadline: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// Close 关闭 Milvus 连接。
func (m *MilvusVectorStore) Close() error {
	if m == nil || m.cli == nil {
		return nil
	}
	return m.cli.Close()
}
