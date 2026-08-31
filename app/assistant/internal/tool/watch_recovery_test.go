package tool

import (
	"context"
	"testing"

	"esx/app/assistant/watch"
)

func TestWatchSideEffectsReconcileCommittedRecovery(t *testing.T) {
	ctx := context.Background()
	watches := watch.NewMapStore()
	registry, err := NewRegistry(Clients{Watch: watches}, []string{CreateWatchTask, DeleteWatchTask})
	if err != nil {
		t.Fatal(err)
	}
	session := &Session{UserID: 4, Recovery: false}
	args := `{"condition_type":"keyword_new_post","target_type":"keyword","target_text":"golang"}`
	first, _, err := registry.Call(ctx, session, CreateWatchTask, "create-call", args)
	if err != nil {
		t.Fatal(err)
	}
	session.Recovery = true
	second, _, err := registry.Call(ctx, session, CreateWatchTask, "create-call", args)
	if err != nil || second != first {
		t.Fatalf("create recovery first=%q second=%q err=%v", first, second, err)
	}
	tasks, err := watches.ListTasks(ctx, 4)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
	deleteArgs := `{"id":1,"expected_version":1}`
	session.Recovery = false
	if _, _, err := registry.Call(ctx, session, DeleteWatchTask, "delete-call", deleteArgs); err != nil {
		t.Fatal(err)
	}
	session.Recovery = true
	if _, _, err := registry.Call(ctx, session, DeleteWatchTask, "delete-call", deleteArgs); err != nil {
		t.Fatalf("delete recovery err=%v", err)
	}
}
