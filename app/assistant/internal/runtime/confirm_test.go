package runtime

import (
	"context"
	"testing"

	"esx/app/assistant/internal/store"
	"esx/pkg/errx"
)

func TestConfirmCAS(t *testing.T) {
	mem := store.NewMemoryStore()
	ctx := context.Background()
	run, err := mem.InsertRun(ctx, store.Run{UserID: 3, SessionID: 1, RequestID: "r", Source: store.SourceUser, Status: store.StatusRunning, Phase: store.PhaseToolExecuting, CreatedAtMs: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = mem.InsertConfirmation(ctx, store.Confirmation{
		UserID: 3, SessionID: 1, RunID: run.ID, CallID: "c1", Tool: "delete_post",
		CanonicalArgsDigest: "abc", TargetRevision: 2, Status: store.ConfirmPending, CreatedAtMs: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Confirm(ctx, mem, 9, run.ID, "c1", true); !errx.Is(err, errx.PermissionDenied) {
		t.Fatalf("other user: %v", err)
	}
	if err := Confirm(ctx, mem, 3, run.ID, "c1", true); err != nil {
		t.Fatal(err)
	}
	if err := Confirm(ctx, mem, 3, run.ID, "c1", false); err == nil {
		t.Fatal("repeat confirm should be invalid")
	}
}
