package runtime

import (
	"context"
	"errors"
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
	if err := engine.requireConfirm(ctx, ctx, run, call, digest); !errors.Is(err, errRunWaiting) {
		t.Fatalf("pending confirmation should yield the worker, err=%v", err)
	}
	confirmation, err := mem.GetConfirmation(ctx, run.ID, call.ID)
	if err != nil || confirmation == nil || confirmation.TargetRevision != 7 || confirmation.CanonicalArgsDigest != digest {
		t.Fatalf("confirmation=%+v err=%v", confirmation, err)
	}
	waiting, err := mem.GetRun(ctx, run.ID)
	if err != nil || waiting.Status != store.StatusWaitingConfirm {
		t.Fatalf("run=%+v err=%v", waiting, err)
	}
	other, err := mem.InsertRun(ctx, store.Run{
		UserID: 8, SessionID: 8, RequestID: "other", Source: store.SourceUser,
		Status: store.StatusQueued, Phase: store.PhaseQueued, CreatedAtMs: store.NowMs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := mem.Claim(ctx, "worker", store.NowMs(), 60_000)
	if err != nil || claimed == nil || claimed.ID != other.ID {
		t.Fatalf("confirmation blocked claim: claimed=%+v err=%v", claimed, err)
	}
	resolved, err := mem.ResolveConfirmation(ctx, run.UserID, run.ID, call.ID, digest, true, store.NowMs())
	if err != nil || resolved == nil || resolved.Status != store.ConfirmApproved {
		t.Fatalf("resolve=%+v err=%v", resolved, err)
	}
	waiting.Status = store.StatusQueued
	waiting.Phase = store.PhaseQueued
	if err := mem.UpdateRun(ctx, *waiting); err != nil {
		t.Fatal(err)
	}
	resumed, err := mem.Claim(ctx, "worker-2", store.NowMs(), 60_000)
	if err != nil || resumed == nil || resumed.ID != run.ID {
		t.Fatalf("resume claim=%+v err=%v", resumed, err)
	}
	if err := engine.requireConfirm(ctx, ctx, resumed, call, digest); err != nil {
		t.Fatal(err)
	}
}
