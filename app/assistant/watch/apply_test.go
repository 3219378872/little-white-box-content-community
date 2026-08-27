package watch

import (
	"testing"

	"esx/pkg/event"
)

func TestApplyPostEventRecordsHitsAndDedupes(t *testing.T) {
	t.Parallel()
	store := NewMapStore()
	authorTask, err := store.Create(t.Context(), Task{
		UserID: 7, ConditionType: AuthorNewPost, TargetType: "author", TargetID: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Create(t.Context(), Task{
		UserID: 8, ConditionType: KeywordNewPost, TargetType: "keyword", TargetText: "怪猎",
	})
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := store.Create(t.Context(), Task{
		UserID: 9, ConditionType: AuthorNewPost, TargetType: "author", TargetID: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateEnabled(t.Context(), 9, disabled.ID, false); err != nil {
		t.Fatal(err)
	}

	created := event.PostEvent{
		EventID: 100, EventTime: 1, Type: event.PostEventCreated,
		PostID: 11, AuthorID: 2, Title: "怪猎更新", Status: 1,
	}
	if err := ApplyPostEvent(t.Context(), store, created); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPostEvent(t.Context(), store, created); err != nil {
		t.Fatal(err)
	}

	hits7, err := store.ListHits(t.Context(), 7, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits7) != 1 || hits7[0].TaskID != authorTask.ID || hits7[0].PostID != 11 {
		t.Fatalf("user 7 hits: %+v", hits7)
	}
	hits8, err := store.ListHits(t.Context(), 8, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits8) != 1 {
		t.Fatalf("user 8 hits: %+v", hits8)
	}
	hits9, err := store.ListHits(t.Context(), 9, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits9) != 0 {
		t.Fatalf("disabled task must not hit: %+v", hits9)
	}
}

func TestApplyPostEventSkipsUnpublishedAndInvalid(t *testing.T) {
	t.Parallel()
	store := NewMapStore()
	if _, err := store.Create(t.Context(), Task{
		UserID: 1, ConditionType: AuthorNewPost, TargetType: "author", TargetID: 2,
	}); err != nil {
		t.Fatal(err)
	}
	draft := event.PostEvent{
		EventID: 2, EventTime: 1, Type: event.PostEventCreated,
		PostID: 11, AuthorID: 2, Title: "草稿", Status: 0,
	}
	if err := ApplyPostEvent(t.Context(), store, draft); err != nil {
		t.Fatal(err)
	}
	hits, err := store.ListHits(t.Context(), 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("draft must not hit: %+v", hits)
	}
	if err := ApplyPostEvent(t.Context(), store, event.PostEvent{}); err != nil {
		t.Fatal(err)
	}
}
