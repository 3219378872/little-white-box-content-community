package logic

import (
	"encoding/json"
	"strconv"
	"testing"

	"esx/pkg/event"
	"mqx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPostOutboxEventRejectsInvalidPayload(t *testing.T) {
	_, err := buildPostOutboxEvent(mqx.TopicPostCreate, event.PostEvent{
		Type: event.PostEventCreated, AuthorID: 1,
	})
	assert.Error(t, err)
}

func TestBuildPostOutboxEventCreatesCanonicalTransportRecord(t *testing.T) {
	record, err := buildPostOutboxEvent(mqx.TopicPostCreate, event.PostEvent{
		Type: event.PostEventCreated, PostID: 1, AuthorID: 2,
	})

	require.NoError(t, err)
	assert.Positive(t, record.ID)
	assert.Equal(t, mqx.TopicPostCreate, record.Topic)
	assert.Equal(t, mqx.TagDefault, record.Tag)
	assert.Equal(t, strconv.FormatInt(record.ID, 10), record.Key)

	var payload event.PostEvent
	require.NoError(t, json.Unmarshal(record.Payload, &payload))
	assert.Equal(t, record.ID, payload.EventID)
	assert.Positive(t, payload.EventTime)
}
