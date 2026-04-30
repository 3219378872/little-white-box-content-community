//go:build integration

package clickhousex

import (
	"context"
	"os"
	"testing"

	"cleanupx"
	"esx/pkg/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/logx"
)

var chEnv *testutil.ClickHouseEnv

func TestMain(m *testing.M) {
	chEnv = testutil.SetupClickHouseEnvM()

	code := m.Run()
	chEnv.Close()
	os.Exit(code)
}

func TestNewClient_PingSucceeds(t *testing.T) {
	client, err := NewClient(chEnv.DSN)
	require.NoError(t, err)
	defer cleanupx.Close(logx.WithContext(context.Background()), "clickhouse client", client)

	assert.NoError(t, client.Ping(context.Background()))
}

func TestNewClient_InvalidDSN_ReturnsError(t *testing.T) {
	_, err := NewClient("clickhouse://invalid:9999/nonexistent?dial_timeout=1s")
	assert.Error(t, err)
}
