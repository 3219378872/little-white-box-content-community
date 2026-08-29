package runtime

import (
	"testing"

	"esx/app/assistant/internal/store"
	"esx/pkg/errx"
)

func TestDispositionStateMachine(t *testing.T) {
	if got := DecideDisposition(nil); got != store.DispositionStarted {
		t.Fatalf("nil: %s", got)
	}
	if got := DecideDisposition(&store.Run{ID: 1, Status: store.StatusDone, Phase: store.PhaseModelRequest}); got != store.DispositionStarted {
		t.Fatalf("done: %s", got)
	}
	if got := DecideDisposition(&store.Run{ID: 1, Status: store.StatusRunning, Phase: store.PhaseModelRequest}); got != store.DispositionRedirected {
		t.Fatalf("model: %s", got)
	}
	if got := DecideDisposition(&store.Run{ID: 1, Status: store.StatusRunning, Phase: store.PhaseToolExecuting}); got != store.DispositionSteered {
		t.Fatalf("tool: %s", got)
	}
	if got := DecideDisposition(&store.Run{ID: 1, Status: store.StatusRunning, Phase: store.PhaseCompact}); got != store.DispositionQueued {
		t.Fatalf("compact: %s", got)
	}
	if err := EnqueueOrReject(31); err != nil {
		t.Fatal(err)
	}
	if err := EnqueueOrReject(32); !errx.Is(err, errx.AgentQueueFull) {
		t.Fatalf("want queue full, got %v", err)
	}
}
