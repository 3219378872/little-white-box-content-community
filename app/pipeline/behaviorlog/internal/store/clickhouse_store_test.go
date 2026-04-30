package store

import (
	"context"
	"testing"
	"time"

	"esx/pkg/event"
	"esx/pkg/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCH(t *testing.T) *testutil.ClickHouseEnv {
	t.Helper()
	return testutil.SetupClickHouseEnv(t, testutil.ClickHouseSchemaPath("behavior_events.sql"))
}

func TestClickHouseStore_Insert_SingleEvent(t *testing.T) {
	chEnv := setupCH(t)
	defer chEnv.Close()

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

func TestClickHouseStore_Insert_DuplicateEventID_Deduped(t *testing.T) {
	chEnv := setupCH(t)
	defer chEnv.Close()

	s := NewClickHouseStore(chEnv.DB)
	e := event.BehaviorEvent{
		EventID:    20001,
		EventTime:  time.Now().UnixMilli(),
		UserID:     42,
		Action:     "like",
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

	err = chEnv.DB.QueryRowContext(context.Background(),
		`SELECT uniqExactMerge(cnt)
		 FROM xbh_analytics.user_action_daily
		 WHERE user_id = 42 AND action = 'like' AND target_type = 'post'`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

func TestClickHouseStore_Insert_InvalidEvent_ReturnsError(t *testing.T) {
	chEnv := setupCH(t)
	defer chEnv.Close()

	s := NewClickHouseStore(chEnv.DB)
	e := event.BehaviorEvent{}

	err := s.Insert(context.Background(), e)
	assert.Error(t, err)
}

func TestClickHouseStore_QueryByUser(t *testing.T) {
	chEnv := setupCH(t)
	defer chEnv.Close()

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
