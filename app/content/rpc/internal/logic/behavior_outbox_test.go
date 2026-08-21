package logic

import (
	"encoding/json"
	"testing"

	"esx/pkg/event"
	"esx/pkg/mqx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildBusinessBehaviorOutboxProducesCanonicalV2Event(t *testing.T) {
	record, err := buildBusinessBehaviorOutbox(event.InteractionEvent{
		UserID: 9, Action: event.BehaviorActionComment,
		TargetID: 11, TargetType: "post", Scene: "content",
	})

	require.NoError(t, err)
	assert.Equal(t, mqx.TopicUserBehaviorV2, record.Topic)
	assert.Equal(t, event.BehaviorActionComment, record.Tag)
	var behavior event.BehaviorEvent
	require.NoError(t, json.Unmarshal(record.Payload, &behavior))
	assert.Equal(t, event.BehaviorSchemaVersion, behavior.SchemaVersion)
	assert.Equal(t, int64(9), behavior.UserID)
	assert.Equal(t, int64(11), behavior.TargetID)
	assert.Equal(t, "business-outbox", behavior.Producer)
}

func TestBuildBusinessBehaviorOutboxRejectsUnsupportedAction(t *testing.T) {
	_, err := buildBusinessBehaviorOutbox(event.InteractionEvent{
		UserID: 9, Action: "unsupported", TargetID: 11, TargetType: "post",
	})
	assert.Error(t, err)
}
