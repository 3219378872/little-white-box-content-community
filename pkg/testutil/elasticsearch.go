package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
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

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        defaultElasticsearchImage,
			ExposedPorts: []string{"9200/tcp"},
			Env: map[string]string{
				"discovery.type": "single-node",
				"cluster.routing.allocation.disk.threshold_enabled": "false",
				"xpack.security.enabled":                            "false",
				"ES_JAVA_OPTS":                                      "-Xms512m -Xmx512m",
			},
			// Wait for the HTTP layer instead of only the TCP port: Elasticsearch
			// binds the port early during startup and can reset connections until
			// the node is ready, which made the integration tests flaky in CI.
			WaitingFor: wait.ForHTTP("/").
				WithPort("9200/tcp").
				WithStartupTimeout(3 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		return nil, fmt.Errorf("elasticsearch container: %w", err)
	}
	address, err := container.PortEndpoint(ctx, "9200/tcp", "http")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, fmt.Errorf("elasticsearch endpoint: %w", err)
	}
	cleanup := func() {
		_ = testcontainers.TerminateContainer(container)
	}
	return &ElasticsearchEnv{
		URL:     address,
		closeFn: cleanup,
	}, nil
}

func (e *ElasticsearchEnv) Close() {
	if e.closeFn != nil {
		e.closeFn()
	}
}
