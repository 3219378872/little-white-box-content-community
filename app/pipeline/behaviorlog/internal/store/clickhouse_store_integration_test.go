//go:build integration

package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"esx/pkg/event"
	"esx/pkg/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var chEnv *testutil.ClickHouseEnv

func TestMain(m *testing.M) {
	chEnv = testutil.SetupClickHouseEnvM(testutil.ClickHouseSchemaPath("xbh_analytics.sql"))
	code := m.Run()
	chEnv.Close()
	os.Exit(code)
}

func canonicalEvent(id, userID, targetID int64, action string) event.BehaviorEvent {
	now := time.Now().UnixMilli()
	return event.BehaviorEvent{
		EventID: id, ClientEventID: fmt.Sprintf("integration-%d", id),
		SchemaVersion: event.BehaviorSchemaVersion, EventTime: now, ReceivedAt: now,
		UserID: userID, Action: action, TargetID: targetID, TargetType: "post",
		Scene: "home", Producer: "integration-test", ClientIP: "10.0.0.1",
	}
}

func TestClickHouseStoreInsertSingleEvent(t *testing.T) {
	s := NewClickHouseStore(chEnv.DB)
	require.NoError(t, s.Insert(context.Background(), canonicalEvent(10001, 42, 999, "like")))

	var count uint64
	err := chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events WHERE event_id = 10001").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)

	// REL-021：行为分析表不得保存完整客户端 IP。
	var storedIP string
	err = chEnv.DB.QueryRowContext(context.Background(),
		"SELECT client_ip FROM xbh_analytics.behavior_events WHERE event_id = 10001").Scan(&storedIP)
	require.NoError(t, err)
	assert.NotEqual(t, "10.0.0.1", storedIP)
	assert.Len(t, storedIP, 64, "client_ip must be a SHA-256 hex digest")
}

func TestClickHouseStoreDuplicateEventIDConverges(t *testing.T) {
	s := NewClickHouseStore(chEnv.DB)
	behavior := canonicalEvent(20001, 42001, 999, "share")
	require.NoError(t, s.Insert(context.Background(), behavior))
	behavior.ReceivedAt++
	require.NoError(t, s.Insert(context.Background(), behavior))

	_, err := chEnv.DB.ExecContext(context.Background(),
		"OPTIMIZE TABLE xbh_analytics.behavior_events FINAL")
	require.NoError(t, err)

	var count uint64
	err = chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events FINAL WHERE event_id = 20001").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

func TestClickHouseStoreUserActionDailyUsesDeduplicatedFacts(t *testing.T) {
	s := NewClickHouseStore(chEnv.DB)
	require.NoError(t, s.Insert(context.Background(), canonicalEvent(21001, 43001, 1, "like")))
	require.NoError(t, s.Insert(context.Background(), canonicalEvent(21002, 43001, 2, "like")))

	var total uint64
	err := chEnv.DB.QueryRowContext(context.Background(), `SELECT sum(cnt)
		FROM xbh_analytics.user_action_daily
		WHERE user_id = 43001 AND action = 'like' AND target_type = 'post'`).Scan(&total)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), total)
}

func TestClickHouseSchemaMatchesV2Contract(t *testing.T) {
	ctx := context.Background()
	rows, err := chEnv.DB.QueryContext(ctx, `SELECT name, type
		FROM system.columns
		WHERE database = 'xbh_analytics' AND table = 'behavior_events'`)
	require.NoError(t, err)
	defer rows.Close()

	columns := make(map[string]string)
	for rows.Next() {
		var name, columnType string
		require.NoError(t, rows.Scan(&name, &columnType))
		columns[name] = columnType
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, "String", columns["client_event_id"])
	assert.Equal(t, "Nullable(Int64)", columns["duration_ms"])

	var engine, partitionKey, sortingKey string
	err = chEnv.DB.QueryRowContext(ctx, `SELECT engine, partition_key, sorting_key
		FROM system.tables
		WHERE database = 'xbh_analytics' AND name = 'behavior_events'`).
		Scan(&engine, &partitionKey, &sortingKey)
	require.NoError(t, err)
	assert.Equal(t, "ReplacingMergeTree", engine)
	assert.Equal(t, "toYYYYMMDD(event_time)", partitionKey)
	assert.Equal(t, "event_id", sortingKey)
}

func TestClickHouseStoreInsertDeadLetter(t *testing.T) {
	s := NewClickHouseStore(chEnv.DB)
	require.NoError(t, s.InsertDeadLetter(context.Background(), DeadLetter{
		MessageID: "bad-message-1", Payload: []byte("bad"), Error: "invalid json",
	}))

	var count uint64
	err := chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_dead_letters WHERE message_id = 'bad-message-1'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

func TestClickHouseStoreInsertInvalidEventReturnsError(t *testing.T) {
	err := NewClickHouseStore(chEnv.DB).Insert(context.Background(), event.BehaviorEvent{})
	assert.Error(t, err)
}

func TestClickHouseStoreQueryByUser(t *testing.T) {
	s := NewClickHouseStore(chEnv.DB)
	for _, behavior := range []event.BehaviorEvent{
		canonicalEvent(30001, 100, 1, "like"),
		canonicalEvent(30002, 100, 2, "favorite"),
		canonicalEvent(30003, 200, 3, "like"),
	} {
		require.NoError(t, s.Insert(context.Background(), behavior))
	}

	var count uint64
	err := chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events FINAL WHERE user_id = 100").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}
