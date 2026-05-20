package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	milvusmodule "github.com/testcontainers/testcontainers-go/modules/milvus"
)

// MilvusEnv 是 testcontainers 启动的 Milvus standalone 实例，供集成测试使用。
type MilvusEnv struct {
	Address string
	closeFn func()
}

// 使用本地已缓存的 v2.5.6 镜像；CI/生产环境可通过覆盖 MILVUS_IMAGE 环境变量调整
const defaultMilvusImage = "milvusdb/milvus:v2.5.6"

func SetupMilvusEnv(t *testing.T) *MilvusEnv {
	t.Helper()
	env, err := setupMilvusEnv()
	require.NoError(t, err)
	return env
}

func SetupMilvusEnvM() *MilvusEnv {
	env, err := setupMilvusEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "SetupMilvusEnvM: %v\n", err)
		os.Exit(1)
	}
	return env
}

func setupMilvusEnv() (*MilvusEnv, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	container, err := milvusmodule.Run(ctx, defaultMilvusImage)
	if err != nil {
		return nil, fmt.Errorf("milvus container: %w", err)
	}
	addr, err := container.ConnectionString(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, fmt.Errorf("milvus connection string: %w", err)
	}
	cleanup := func() {
		_ = testcontainers.TerminateContainer(container)
	}
	return &MilvusEnv{Address: addr, closeFn: cleanup}, nil
}

func (e *MilvusEnv) Close() {
	if e.closeFn != nil {
		e.closeFn()
	}
}
