package event

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validBehaviorEvent() BehaviorEvent {
	return BehaviorEvent{
		EventID: 100001, ClientEventID: "client-1", SchemaVersion: BehaviorSchemaVersion,
		EventTime: 1714300000000, ReceivedAt: 1714300000100, UserID: 42,
		Action: BehaviorActionExposure, TargetID: 999, TargetType: "post",
		Scene: "home", RequestID: "request-1", Position: new(int32(3)),
		Producer: "behavior-rpc", ClientIP: "10.0.0.1", ClientVersion: "2.0.0",
	}
}

func TestBehaviorEventJSONRoundTrip(t *testing.T) {
	e := validBehaviorEvent()
	data, err := json.Marshal(e)
	require.NoError(t, err)

	var got BehaviorEvent
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, e, got)
}

func TestBehaviorEventValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*BehaviorEvent)
		wantErr string
	}{
		{name: "valid"},
		{name: "anonymous accepted", mutate: func(e *BehaviorEvent) { e.UserID = 0; e.AnonymousID = "device-1" }},
		{name: "identity required", mutate: func(e *BehaviorEvent) { e.UserID = 0 }, wantErr: "user_id or anonymous_id"},
		{name: "client id required", mutate: func(e *BehaviorEvent) { e.ClientEventID = "" }, wantErr: "client_event_id"},
		{name: "schema required", mutate: func(e *BehaviorEvent) { e.SchemaVersion = 1 }, wantErr: "schema_version"},
		{name: "exposure request required", mutate: func(e *BehaviorEvent) { e.RequestID = "" }, wantErr: "request_id"},
		{name: "exposure position required", mutate: func(e *BehaviorEvent) { e.Position = nil }, wantErr: "position"},
		{name: "duration rejected for like", mutate: func(e *BehaviorEvent) { e.Action = BehaviorActionLike; e.DurationMs = new(int64(10)) }, wantErr: "not allowed"},
		{name: "duration required for dwell", mutate: func(e *BehaviorEvent) { e.Action = BehaviorActionDwell }, wantErr: "duration_ms is required"},
		{name: "duration accepted for dwell", mutate: func(e *BehaviorEvent) { e.Action = BehaviorActionDwell; e.DurationMs = new(int64(1000)) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := validBehaviorEvent()
			if tt.mutate != nil {
				tt.mutate(&e)
			}
			err := e.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestBehaviorEventValidateClientSubmitted(t *testing.T) {
	base := validBehaviorEvent()
	for _, action := range []string{
		BehaviorActionExposure, BehaviorActionClick, BehaviorActionDwell,
		BehaviorActionPlay, BehaviorActionView, BehaviorActionShare,
		BehaviorActionHide, BehaviorActionDislike,
	} {
		t.Run("client-allowed-"+action, func(t *testing.T) {
			e := base
			e.Action = action
			if _, ok := map[string]struct{}{
				BehaviorActionDwell: {}, BehaviorActionPlay: {}, BehaviorActionView: {},
			}[action]; ok {
				e.DurationMs = new(int64(1000))
			}
			require.NoError(t, e.ValidateClientSubmitted())
		})
	}

	for _, action := range []string{
		BehaviorActionLike, BehaviorActionUnlike, BehaviorActionFavorite,
		BehaviorActionUnfavorite, BehaviorActionComment, BehaviorActionFollow,
		BehaviorActionUnfollow,
	} {
		t.Run("client-rejected-"+action, func(t *testing.T) {
			e := base
			e.Action = action
			assert.ErrorContains(t, e.ValidateClientSubmitted(), "not allowed from clients")
		})
	}
}

func TestBehaviorEventExposurePositionMustStartFromOne(t *testing.T) {
	e := validBehaviorEvent()
	zero := int32(0)
	e.Position = &zero
	assert.ErrorContains(t, e.Validate(), "position must start from 1")
}

func TestDeterministicBehaviorEventID(t *testing.T) {
	first := DeterministicBehaviorEventID("client-1")
	assert.Positive(t, first)
	assert.Equal(t, first, DeterministicBehaviorEventID("client-1"))
	assert.NotEqual(t, first, DeterministicBehaviorEventID("client-2"))
}

func TestBehaviorEventEventIDString(t *testing.T) {
	assert.Equal(t, "123456789", BehaviorEvent{EventID: 123456789}.EventIDString())
}

func FuzzBehaviorEventJSONNeverPanics(f *testing.F) {
	for _, seed := range []string{`{}`, `null`, `{"user_id":"wrong-type"}`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, payload string) {
		var behavior BehaviorEvent
		if json.Unmarshal([]byte(payload), &behavior) == nil {
			_ = behavior.Validate()
		}
	})
}
