//go:build integration

package logic

import (
	"context"
	"fmt"
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
	d := dedup.NewExactDedup(svc.NewRedisExactStore(testEnv.Redis), 3600)
	return NewRecorder(s, d)
}

func TestIntegration_FullPipeline_EventPersistedInClickHouse(t *testing.T) {
	recorder := newIntegrationRecorder()
	e := event.BehaviorEvent{
		EventID: 99001, ClientEventID: "integration-99001", SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 1714300000000, ReceivedAt: 1714300000100, UserID: 42,
		Action: "like", TargetID: 999, TargetType: "post", Scene: "home", Producer: "test",
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
		EventID: 99002, ClientEventID: "integration-99002", SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 1714300000000, ReceivedAt: 1714300000100, UserID: 42,
		Action: "favorite", TargetID: 888, TargetType: "post", Producer: "test",
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
			EventID: a.eventID, ClientEventID: fmt.Sprintf("integration-%d", a.eventID),
			SchemaVersion: event.BehaviorSchemaVersion, EventTime: now, ReceivedAt: now,
			UserID: 50, Action: a.action, TargetID: a.targetID, TargetType: "post", Producer: "test",
		}
		require.NoError(t, recorder.Process(context.Background(), e, MessageMeta{}))
	}

	var count uint64
	err := chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events WHERE user_id = 50 AND event_id >= 99010 AND event_id <= 99013").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(4), count)
}
