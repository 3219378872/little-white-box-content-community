package event

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostEvent_JSONRoundTrip(t *testing.T) {
	e := PostEvent{
		EventID:     7001,
		EventTime:   1714300000000,
		Type:        PostEventCreated,
		PostID:      999,
		AuthorID:    42,
		Title:       "hello world",
		BodyExcerpt: "lorem ipsum",
		CategoryID:  3,
		Tags:        []string{"游戏", "科技"},
	}
	data, err := json.Marshal(e)
	require.NoError(t, err)

	var got PostEvent
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, e, got)
}

func TestPostEvent_Validate(t *testing.T) {
	base := PostEvent{
		EventID: 1, EventTime: 1, Type: PostEventCreated,
		PostID: 100, AuthorID: 1,
	}
	require.NoError(t, base.Validate())

	cases := []struct {
		name    string
		mutate  func(e *PostEvent)
		wantSub string
	}{
		{"missing event_id", func(e *PostEvent) { e.EventID = 0 }, "event_id"},
		{"missing post_id", func(e *PostEvent) { e.PostID = 0 }, "post_id"},
		{"missing type", func(e *PostEvent) { e.Type = "" }, "type"},
		{"unknown type", func(e *PostEvent) { e.Type = "post.weird" }, "unknown"},
		{"missing author on create", func(e *PostEvent) { e.AuthorID = 0 }, "author_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := base
			tc.mutate(&e)
			err := e.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantSub)
		})
	}
}

func TestPostEvent_Validate_DeleteAllowsZeroAuthor(t *testing.T) {
	e := PostEvent{EventID: 1, EventTime: 1, Type: PostEventDeleted, PostID: 100}
	assert.NoError(t, e.Validate())
}

func TestInteractionEvent_JSONRoundTrip(t *testing.T) {
	e := InteractionEvent{
		EventID:    9001,
		EventTime:  1714300000000,
		UserID:     42,
		Action:     "like",
		TargetID:   999,
		TargetType: "post",
		Scene:      "home",
		ClientIP:   "10.0.0.1",
	}
	data, err := json.Marshal(e)
	require.NoError(t, err)

	var got InteractionEvent
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, e, got)
}

func TestInteractionEvent_Validate(t *testing.T) {
	base := InteractionEvent{
		EventID: 1, UserID: 42, Action: "like",
		TargetID: 100, TargetType: "post",
	}
	require.NoError(t, base.Validate())

	cases := []struct {
		name    string
		mutate  func(*InteractionEvent)
		wantSub string
	}{
		{"missing event_id", func(e *InteractionEvent) { e.EventID = 0 }, "event_id"},
		{"missing user_id", func(e *InteractionEvent) { e.UserID = 0 }, "user_id"},
		{"missing action", func(e *InteractionEvent) { e.Action = "" }, "action"},
		{"missing target_id", func(e *InteractionEvent) { e.TargetID = 0 }, "target_id"},
		{"missing target_type", func(e *InteractionEvent) { e.TargetType = "" }, "target_type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := base
			tc.mutate(&e)
			err := e.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantSub)
		})
	}
}

func TestInteractionEvent_ToBehaviorEvent(t *testing.T) {
	e := InteractionEvent{
		EventID: 1, EventTime: 2, UserID: 42, Action: "favorite",
		TargetID: 100, TargetType: "post", Scene: "discover", ClientIP: "1.1.1.1",
	}
	be := e.ToBehaviorEvent(500)
	assert.Equal(t, int64(1), be.EventID)
	assert.Equal(t, int64(2), be.EventTime)
	assert.Equal(t, int64(42), be.UserID)
	assert.Equal(t, "favorite", be.Action)
	assert.Equal(t, int64(100), be.TargetID)
	assert.Equal(t, "post", be.TargetType)
	assert.Equal(t, int32(500), be.Duration)
	assert.Equal(t, "discover", be.Scene)
	assert.Equal(t, "1.1.1.1", be.ClientIP)
}
