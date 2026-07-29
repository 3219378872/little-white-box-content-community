package logic

import (
	"encoding/json"
	"testing"

	"esx/pkg/event"
	"mqx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFollowOutboxEventProducesCanonicalBehavior(t *testing.T) {
	record, err := followOutboxEvent(10, 20, event.BehaviorActionFollow)

	require.NoError(t, err)
	assert.Equal(t, mqx.TopicUserBehaviorV2, record.Topic)
	var behavior event.BehaviorEvent
	require.NoError(t, json.Unmarshal(record.Payload, &behavior))
	assert.Equal(t, int64(10), behavior.UserID)
	assert.Equal(t, int64(20), behavior.TargetID)
	assert.Equal(t, "user", behavior.TargetType)
}
