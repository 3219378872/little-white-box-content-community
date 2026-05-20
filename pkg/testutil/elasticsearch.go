package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	esmodule "github.com/testcontainers/testcontainers-go/modules/elasticsearch"
)

// ElasticsearchEnv 是 testcontainers 启动的 ES 单节点环境，供集成测试使用。
type ElasticsearchEnv struct {
	URL      string
	Username string
	Password string
	CACert   []byte
	closeFn  func()
}

const defaultElasticsearchImage = "docker.elastic.co/elasticsearch/elasticsearch:8.8.0"

func SetupElasticsearchEnv(t *testing.T) *ElasticsearchEnv {
	t.Helper()
	env, err := setupElasticsearchEnv()
	require.NoError(t, err)
	return env
}

func SetupElasticsearchEnvM() *ElasticsearchEnv {
	env, err := setupElasticsearchEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "SetupElasticsearchEnvM: %v\n", err)
		os.Exit(1)
	}
	return env
}

func setupElasticsearchEnv() (*ElasticsearchEnv, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := esmodule.Run(ctx, defaultElasticsearchImage)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch container: %w", err)
	}
	cleanup := func() {
		_ = testcontainers.TerminateContainer(container)
	}
	return &ElasticsearchEnv{
		URL:      container.Settings.Address,
		Username: container.Settings.Username,
		Password: container.Settings.Password,
		CACert:   container.Settings.CACert,
		closeFn:  cleanup,
	}, nil
}

func (e *ElasticsearchEnv) Close() {
	if e.closeFn != nil {
		e.closeFn()
	}
}
