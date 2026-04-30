//go:build integration

package logic

import (
	"context"
	"os"
	"testing"
	"time"

	"esx/app/pipeline/behaviorlog/internal/dedup"
	"esx/app/pipeline/behaviorlog/internal/store"
	"esx/app/pipeline/behaviorlog/internal/svc"
	"esx/pkg/event"
	"esx/pkg/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	chEnv   *testutil.ClickHouseEnv
	testEnv *testutil.TestEnv
)

func TestMain(m *testing.M) {
	chEnv = testutil.SetupClickHouseEnvM(testutil.ClickHouseSchemaPath("xbh_analytics.sql"))
	testEnv = testutil.SetupTestEnvM("xbh_test_behaviorlog", testutil.SchemaPath("xbh_user.sql"))

	code := m.Run()
	chEnv.Close()
	testEnv.Close()
	os.Exit(code)
}

func newIntegrationRecorder() *Recorder {
	s := store.NewClickHouseStore(chEnv.DB)
	d := dedup.NewBloomDedup(svc.NewRedisBloomStore(testEnv.Redis), 1024)
	return NewRecorder(s, d)
}

func TestIntegration_FullPipeline_EventPersistedInClickHouse(t *testing.T) {
	recorder := newIntegrationRecorder()
	e := event.BehaviorEvent{
		EventID:    99001,
		EventTime:  1714300000000,
		UserID:     42,
		Action:     "like",
		TargetID:   999,
		TargetType: "post",
		Scene:      "home",
	}

	require.NoError(t, recorder.Process(context.Background(), e, MessageMeta{}))

	var count uint64
	err := chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events WHERE event_id = 99001").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

func TestIntegration_DuplicateEvent_FilteredByBloom(t *testing.T) {
	recorder := newIntegrationRecorder()
	e := event.BehaviorEvent{
		EventID:    99002,
		EventTime:  1714300000000,
		UserID:     42,
		Action:     "favorite",
		TargetID:   888,
		TargetType: "post",
	}

	require.NoError(t, recorder.Process(context.Background(), e, MessageMeta{}))
	require.NoError(t, recorder.Process(context.Background(), e, MessageMeta{}))

	var count uint64
	err := chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events WHERE event_id = 99002").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

func TestIntegration_MultipleActions_AllPersisted(t *testing.T) {
	recorder := newIntegrationRecorder()
	now := time.Now().UnixMilli()
	actions := []struct {
		eventID  int64
		action   string
		targetID int64
	}{
		{99010, "like", 100},
		{99011, "favorite", 101},
		{99012, "comment", 102},
		{99013, "follow", 200},
	}

	for _, a := range actions {
		e := event.BehaviorEvent{
			EventID: a.eventID, EventTime: now,
			UserID: 50, Action: a.action,
			TargetID: a.targetID, TargetType: "post",
		}
		require.NoError(t, recorder.Process(context.Background(), e, MessageMeta{}))
	}

	var count uint64
	err := chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events WHERE user_id = 50 AND event_id >= 99010 AND event_id <= 99013").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(4), count)
}
