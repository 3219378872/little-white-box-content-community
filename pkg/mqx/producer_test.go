package mqx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMessage(t *testing.T) {
	msg, err := BuildMessage(Message{
		Topic: "events", Tag: "v2", Key: "client-1", Body: []byte(`{"ok":true}`),
		Properties: map[string]string{"trace_id": "trace-1"},
	})
	require.NoError(t, err)
	assert.Equal(t, "events", msg.Topic)
	assert.Equal(t, "v2", msg.GetTags())
	assert.Equal(t, "client-1", msg.GetKeys())
	assert.Equal(t, "trace-1", msg.GetProperty("trace_id"))
}

func TestBuildMessageRejectsMissingRequiredFields(t *testing.T) {
	_, err := BuildMessage(Message{Body: []byte("payload")})
	assert.ErrorContains(t, err, "topic")

	_, err = BuildMessage(Message{Topic: "events"})
	assert.ErrorContains(t, err, "body")
}
