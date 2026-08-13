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

// REL-020：去标识聚合写入 daily_aggregates，按 event_id 收敛后不重复累计，
// 重复执行幂等，目标表 TTL 为 365 天。
func TestClickHouseStoreAggregateDailyDedupesAndIsIdempotent(t *testing.T) {
	s := NewClickHouseStore(chEnv.DB)
	ctx := context.Background()

	dayA := time.Now().AddDate(0, 0, -10) // 窗口内：10 天前
	dayB := time.Now().AddDate(0, 0, -9)  // 窗口内：9 天前

	ev1 := canonicalEvent(30001, 42, 999, "like")
	ev1.EventTime = dayA.UnixMilli()
	ev1.ReceivedAt = dayA.UnixMilli()
	dup1 := ev1
	dup1.ReceivedAt = dayA.UnixMilli() + 1000 // 同 event_id 的 at-least-once 重投

	ev2 := canonicalEvent(30002, 42, 999, "click")
	ev2.EventTime = dayA.UnixMilli()
	ev2.ReceivedAt = dayA.UnixMilli()

	ev3 := canonicalEvent(30003, 7, 111, "like")
	ev3.EventTime = dayB.UnixMilli()
	ev3.ReceivedAt = dayB.UnixMilli()

	// 窗口外事件（今天）：不应进入聚合窗口。
	outside := canonicalEvent(30004, 99, 555, "like")

	for _, e := range []event.BehaviorEvent{ev1, dup1, ev2, ev3, outside} {
		require.NoError(t, s.Insert(ctx, e))
	}

	from := time.Date(dayA.Year(), dayA.Month(), dayA.Day(), 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 2) // 覆盖 dayA、dayB

	count, err := s.AggregateDaily(ctx, from, to)
	require.NoError(t, err)
	// 去重后唯一事件：dayA like(1) + dayA click(1) + dayB like(1) = 3
	assert.Equal(t, int64(3), count, "aggregate must dedupe by event_id and exclude window outside")

	// 幂等：重复执行不重复累计（ReplacingMergeTree(aggregated_at)）
	count2, err := s.AggregateDaily(ctx, from, to)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count2, "re-running the aggregate must stay idempotent")

	// REL-020：365 天自动删除由表 TTL 承担（用 create_table_query 断言，跨版本稳定）
	var createQuery string
	err = chEnv.DB.QueryRowContext(ctx,
		`SELECT create_table_query FROM system.tables WHERE database = 'xbh_analytics' AND name = 'daily_aggregates'`,
	).Scan(&createQuery)
	require.NoError(t, err)
	assert.Contains(t, createQuery, "toIntervalDay(365)", "daily_aggregates TTL must retain 365 days")
}
