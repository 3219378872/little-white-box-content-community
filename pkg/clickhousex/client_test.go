package clickhousex

import (
	"context"
	"testing"

	"esx/pkg/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_PingSucceeds(t *testing.T) {
	chEnv := testutil.SetupClickHouseEnv(t)
	defer chEnv.Close()

	client, err := NewClient(chEnv.DSN)
	require.NoError(t, err)
	defer client.Close()

	assert.NoError(t, client.Ping(context.Background()))
}

func TestNewClient_InvalidDSN_ReturnsError(t *testing.T) {
	_, err := NewClient("clickhouse://invalid:9999/nonexistent?dial_timeout=1s")
	assert.Error(t, err)
}
