package logic

import (
	"encoding/json"
	"testing"

	"esx/pkg/event"
	"esx/pkg/mqx"
	"esx/pkg/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	_ = util.InitSnowflake(3, 1)
}

func TestInteractionOutboxEventUsesCanonicalV2Topic(t *testing.T) {
	record, err := interactionOutboxEvent(7, 9, "post", event.BehaviorActionLike)

	require.NoError(t, err)
	assert.Equal(t, mqx.TopicUserBehaviorV2, record.Topic)
	assert.Equal(t, event.BehaviorActionLike, record.Tag)
	var behavior event.BehaviorEvent
	require.NoError(t, json.Unmarshal(record.Payload, &behavior))
	assert.Equal(t, int64(7), behavior.UserID)
	assert.Equal(t, int64(9), behavior.TargetID)
	assert.Equal(t, "business-outbox", behavior.Producer)
}

func TestTargetTypeName(t *testing.T) {
	assert.Equal(t, "post", targetTypeName(1))
	assert.Equal(t, "comment", targetTypeName(2))
}
