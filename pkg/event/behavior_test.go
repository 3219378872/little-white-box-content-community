package event

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBehaviorEvent_JSONRoundTrip(t *testing.T) {
	e := BehaviorEvent{
		EventID:    100001,
		EventTime:  1714300000000,
		UserID:     42,
		Action:     "like",
		TargetID:   999,
		TargetType: "post",
		Duration:   0,
		Scene:      "home",
		ClientIP:   "10.0.0.1",
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)

	var got BehaviorEvent
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, e, got)
}

func TestBehaviorEvent_Validate_Valid(t *testing.T) {
	e := BehaviorEvent{
		EventID:    1,
		UserID:     42,
		Action:     "like",
		TargetID:   100,
		TargetType: "post",
	}
	assert.NoError(t, e.Validate())
}

func TestBehaviorEvent_Validate_MissingUserID(t *testing.T) {
	e := BehaviorEvent{EventID: 1, Action: "like", TargetID: 100, TargetType: "post"}
	assert.ErrorContains(t, e.Validate(), "user_id")
}

func TestBehaviorEvent_Validate_MissingAction(t *testing.T) {
	e := BehaviorEvent{EventID: 1, UserID: 42, TargetID: 100, TargetType: "post"}
	assert.ErrorContains(t, e.Validate(), "action")
}

func TestBehaviorEvent_Validate_MissingTargetID(t *testing.T) {
	e := BehaviorEvent{EventID: 1, UserID: 42, Action: "like", TargetType: "post"}
	assert.ErrorContains(t, e.Validate(), "target_id")
}

func TestBehaviorEvent_Validate_MissingTargetType(t *testing.T) {
	e := BehaviorEvent{EventID: 1, UserID: 42, Action: "like", TargetID: 100}
	assert.ErrorContains(t, e.Validate(), "target_type")
}

func TestBehaviorEvent_EventIDString(t *testing.T) {
	e := BehaviorEvent{EventID: 123456789}
	assert.Equal(t, "123456789", e.EventIDString())
}
