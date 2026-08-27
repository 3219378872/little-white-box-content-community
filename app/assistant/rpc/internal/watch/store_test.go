package watch

import (
	"testing"

	"esx/pkg/errx"
	"esx/pkg/event"
)

func TestMatchRules(t *testing.T) {
	t.Parallel()
	created := event.PostEvent{Type: event.PostEventCreated, PostID: 11, AuthorID: 2, Title: "怪猎更新", Tags: []string{"MHW"}, Status: 1}
	if ok, _ := Match(Task{Enabled: true, ConditionType: AuthorNewPost, TargetType: "author", TargetID: 2}, created); !ok {
		t.Fatal("author_new_post")
	}
	if ok, _ := Match(Task{Enabled: true, ConditionType: TagNewPost, TargetType: "tag", TargetText: "mhw"}, created); !ok {
		t.Fatal("tag_new_post")
	}
	if ok, _ := Match(Task{Enabled: true, ConditionType: KeywordNewPost, TargetType: "keyword", TargetText: "怪猎"}, created); !ok {
		t.Fatal("keyword_new_post")
	}
	updated := event.PostEvent{Type: event.PostEventUpdated, PostID: 9, Status: 1}
	if ok, _ := Match(Task{Enabled: true, ConditionType: PostRevised, TargetType: "post", TargetID: 9}, updated); !ok {
		t.Fatal("post_revised")
	}
	if ok, _ := Match(Task{Enabled: false, ConditionType: AuthorNewPost, TargetType: "author", TargetID: 2}, created); ok {
		t.Fatal("disabled task must not hit")
	}
}

func TestCreateRejectsUnknownConditionAndDuplicates(t *testing.T) {
	store := NewMapStore()
	_, err := store.Create(t.Context(), Task{UserID: 2, ConditionType: "price_drop", TargetType: "post", TargetID: 1})
	if !errx.Is(err, errx.ParamError) {
		t.Fatalf("unknown: %v", err)
	}
	task, err := store.Create(t.Context(), Task{UserID: 2, ConditionType: AuthorNewPost, TargetType: "author", TargetID: 8})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Create(t.Context(), Task{UserID: 2, ConditionType: AuthorNewPost, TargetType: "author", TargetID: 8})
	if !errx.Is(err, errx.IdempotencyConflict) {
		t.Fatalf("dup: %v", err)
	}
	if task.ID == 0 || !task.Enabled {
		t.Fatalf("%+v", task)
	}
}
