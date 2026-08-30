package runtime

import (
	"context"
	"testing"
	"time"

	"esx/app/assistant/internal/canonical"
	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
)

func TestDeleteConfirmationBindsConcreteTargetRevision(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	mem := store.NewMemoryStore()
	queued, err := mem.InsertRun(ctx, store.Run{
		UserID: 3, SessionID: 4, RequestID: "delete-request", Source: store.SourceUser,
		Status: store.StatusQueued, Phase: store.PhaseToolExecuting, ConsentVersion: 2, InputVersion: 1,
		CreatedAtMs: store.NowMs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := mem.Claim(ctx, "worker", store.NowMs(), 60_000)
	if err != nil || run == nil || run.ID != queued.ID {
		t.Fatalf("claim=%+v err=%v", run, err)
	}
	call := llm.ToolCall{ID: "delete-call", Name: tool.DeletePost, Arguments: `{"post_id":9,"expected_revision":7}`, Prepared: true}
	digest, err := canonical.DigestArgs(call.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{Store: mem}
	result := make(chan error, 1)
	go func() { result <- engine.requireConfirm(ctx, ctx, run, call, digest) }()
	var confirmation *store.Confirmation
	deadline := time.NewTimer(500 * time.Millisecond)
	defer deadline.Stop()
	for confirmation == nil {
		select {
		case <-deadline.C:
			t.Fatal("confirmation was not persisted")
		default:
			confirmation, err = mem.GetConfirmation(ctx, run.ID, call.ID)
			if err != nil {
				t.Fatal(err)
			}
			if confirmation == nil {
				time.Sleep(time.Millisecond)
			}
		}
	}
	if confirmation.TargetRevision != 7 || confirmation.CanonicalArgsDigest != digest {
		t.Fatalf("confirmation=%+v", confirmation)
	}
	resolved, err := mem.ResolveConfirmation(ctx, run.UserID, run.ID, call.ID, digest, true, store.NowMs())
	if err != nil || resolved == nil || resolved.Status != store.ConfirmApproved {
		t.Fatalf("resolve=%+v err=%v", resolved, err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("confirmation waiter did not resume")
	}
}
