//go:build integration

package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"esx/app/pipeline/behaviorlog/internal/dedup"
	"esx/app/pipeline/behaviorlog/internal/store"
	"esx/pkg/event"
	"esx/pkg/testutil"
	"util"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	chEnv   *testutil.ClickHouseEnv
	testEnv *testutil.TestEnv
)

func TestMain(m *testing.M) {
	_ = util.InitSnowflake(1, 1)

	chEnv = testutil.SetupClickHouseEnvM(testutil.ClickHouseSchemaPath("behavior_events.sql"))
	defer chEnv.Close()

	testEnv = testutil.SetupTestEnvM("xbh_test_behaviorlog", testutil.SchemaPath("xbh_user.sql"))
	defer testEnv.Close()

	os.Exit(m.Run())
}

func TestIntegration_FullPipeline_EventPersistedInClickHouse(t *testing.T) {
	s := store.NewClickHouseStore(chEnv.DB)
	d := dedup.NewBloomDedup(testEnv.Redis, 1024)
	handler := MakeBehaviorHandler(s, d)

	msg := &primitive.MessageExt{
		Message: primitive.Message{
			Body: []byte(`{
				"event_id": 99001,
				"event_time": 1714300000000,
				"user_id": 42,
				"action": "like",
				"target_id": 999,
				"target_type": "post",
				"scene": "home"
			}`),
		},
		MsgId: "integration-msg-1",
	}

	result, err := handler(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(t, consumer.ConsumeSuccess, result)

	var count uint64
	err = chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events WHERE event_id = 99001").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

func TestIntegration_DuplicateEvent_FilteredByBloom(t *testing.T) {
	s := store.NewClickHouseStore(chEnv.DB)
	d := dedup.NewBloomDedup(testEnv.Redis, 1024)
	handler := MakeBehaviorHandler(s, d)

	body := []byte(`{
		"event_id": 99002,
		"event_time": 1714300000000,
		"user_id": 42,
		"action": "favorite",
		"target_id": 888,
		"target_type": "post"
	}`)

	msg1 := &primitive.MessageExt{Message: primitive.Message{Body: body}, MsgId: "dup-1"}
	msg2 := &primitive.MessageExt{Message: primitive.Message{Body: body}, MsgId: "dup-2"}

	result1, err := handler(context.Background(), msg1)
	require.NoError(t, err)
	assert.Equal(t, consumer.ConsumeSuccess, result1)

	result2, err := handler(context.Background(), msg2)
	require.NoError(t, err)
	assert.Equal(t, consumer.ConsumeSuccess, result2)

	var count uint64
	err = chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events WHERE event_id = 99002").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

func TestIntegration_MultipleActions_AllPersisted(t *testing.T) {
	s := store.NewClickHouseStore(chEnv.DB)
	d := dedup.NewBloomDedup(testEnv.Redis, 1024)
	handler := MakeBehaviorHandler(s, d)

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
		body, err := json.Marshal(e)
		require.NoError(t, err)
		msg := &primitive.MessageExt{
			Message: primitive.Message{Body: body},
			MsgId:   fmt.Sprintf("multi-%d", a.eventID),
		}
		result, err := handler(context.Background(), msg)
		require.NoError(t, err)
		assert.Equal(t, consumer.ConsumeSuccess, result)
	}

	var count uint64
	err := chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events WHERE user_id = 50 AND event_id >= 99010 AND event_id <= 99013").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(4), count)
}
