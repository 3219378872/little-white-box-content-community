//go:build integration

package store

import (
	"context"
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

func TestClickHouseStore_Insert_SingleEvent(t *testing.T) {
	s := NewClickHouseStore(chEnv.DB)
	e := event.BehaviorEvent{
		EventID:    10001,
		EventTime:  time.Now().UnixMilli(),
		UserID:     42,
		Action:     "like",
		TargetID:   999,
		TargetType: "post",
		Duration:   0,
		Scene:      "home",
		ClientIP:   "10.0.0.1",
	}

	err := s.Insert(context.Background(), e)
	require.NoError(t, err)

	var count uint64
	err = chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events WHERE event_id = 10001").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

// 主表使用 ReplacingMergeTree(event_time)，重复 event_id 在 OPTIMIZE FINAL 后只保留一条。
// MV 由 INSERT 触发，不做事件级去重 — 去重责任在消费端 Bloom Filter + 主表 FINAL，
// 详见 spec §3.4 / §4.1。
func TestClickHouseStore_Insert_DuplicateEventID_MainTableDeduped(t *testing.T) {
	s := NewClickHouseStore(chEnv.DB)
	e := event.BehaviorEvent{
		EventID:    20001,
		EventTime:  time.Now().UnixMilli(),
		UserID:     42001,
		Action:     "share",
		TargetID:   999,
		TargetType: "post",
	}

	require.NoError(t, s.Insert(context.Background(), e))
	require.NoError(t, s.Insert(context.Background(), e))

	_, err := chEnv.DB.ExecContext(context.Background(),
		"OPTIMIZE TABLE xbh_analytics.behavior_events FINAL")
	require.NoError(t, err)

	var count uint64
	err = chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events FINAL WHERE event_id = 20001").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

// SummingMergeTree 在 merge 时按 (user_id, action, target_type, date) 自动累加 cnt 列；
// 验证两条同 key 事件 → sum(cnt) = 2（MV 不做事件级去重）。
func TestClickHouseStore_UserActionDaily_SumsCount(t *testing.T) {
	s := NewClickHouseStore(chEnv.DB)
	now := time.Now().UnixMilli()
	e1 := event.BehaviorEvent{
		EventID: 21001, EventTime: now, UserID: 43001,
		Action: "view", TargetID: 1, TargetType: "post",
	}
	e2 := e1
	e2.EventID = 21002

	require.NoError(t, s.Insert(context.Background(), e1))
	require.NoError(t, s.Insert(context.Background(), e2))

	_, err := chEnv.DB.ExecContext(context.Background(),
		"OPTIMIZE TABLE xbh_analytics.user_action_daily FINAL")
	require.NoError(t, err)

	var total uint64
	err = chEnv.DB.QueryRowContext(context.Background(),
		`SELECT sum(cnt)
		 FROM xbh_analytics.user_action_daily
		 WHERE user_id = 43001 AND action = 'view' AND target_type = 'post'`).Scan(&total)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), total)
}

// 验证 ClickHouse DDL 与 spec §4.1 一致：engine / partition / sort key 不可漂移。
// 用 SHOW CREATE TABLE 而非 system.tables，因为 MV 的 partition/sort 元数据存储于 inner 表。
func TestClickHouseSchema_MatchesSpec(t *testing.T) {
	cases := []struct {
		table       string
		wantSubstrs []string
	}{
		{
			table: "behavior_events",
			wantSubstrs: []string{
				"ReplacingMergeTree(event_time)",
				"PARTITION BY toYYYYMMDD(event_time)",
				"ORDER BY (user_id, action, event_time, event_id)",
			},
		},
		{
			table: "user_action_daily",
			wantSubstrs: []string{
				"SummingMergeTree",
				"ORDER BY (user_id, action, target_type, date)",
				"count() AS cnt",
			},
		},
		{
			table: "behavior_events_by_time",
			wantSubstrs: []string{
				"ReplacingMergeTree(event_time)",
				"PARTITION BY toYYYYMMDD(event_time)",
				"ORDER BY (event_time, user_id, event_id)",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.table, func(t *testing.T) {
			var ddl string
			err := chEnv.DB.QueryRowContext(context.Background(),
				"SHOW CREATE TABLE xbh_analytics."+tc.table).Scan(&ddl)
			require.NoError(t, err, "table %s should exist", tc.table)
			for _, want := range tc.wantSubstrs {
				assert.Contains(t, ddl, want, "DDL missing %q\nfull DDL:\n%s", want, ddl)
			}
		})
	}
}

func TestClickHouseStore_Insert_InvalidEvent_ReturnsError(t *testing.T) {
	s := NewClickHouseStore(chEnv.DB)
	e := event.BehaviorEvent{}

	err := s.Insert(context.Background(), e)
	assert.Error(t, err)
}

func TestClickHouseStore_QueryByUser(t *testing.T) {
	s := NewClickHouseStore(chEnv.DB)
	now := time.Now().UnixMilli()

	events := []event.BehaviorEvent{
		{EventID: 30001, EventTime: now, UserID: 100, Action: "like", TargetID: 1, TargetType: "post"},
		{EventID: 30002, EventTime: now, UserID: 100, Action: "favorite", TargetID: 2, TargetType: "post"},
		{EventID: 30003, EventTime: now, UserID: 200, Action: "like", TargetID: 3, TargetType: "post"},
	}
	for _, e := range events {
		require.NoError(t, s.Insert(context.Background(), e))
	}

	var count uint64
	err := chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events WHERE user_id = 100").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}
