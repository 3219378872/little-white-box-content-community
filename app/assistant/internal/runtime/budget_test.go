package runtime

import (
	"context"
	"testing"

	"esx/app/assistant/internal/store"
)

func TestBudgetOncePerDimension(t *testing.T) {
	mem := store.NewMemoryStore()
	run := store.Run{ID: 9, Rounds: 35, OutputTokens: 120_000, LastActivityAtMs: store.NowMs()}
	first, err := RecordAlarms(context.Background(), mem, run, store.NowMs())
	if err != nil || first == "" {
		t.Fatalf("first alarm: %q err=%v", first, err)
	}
	second, err := RecordAlarms(context.Background(), mem, run, store.NowMs())
	if err != nil {
		t.Fatal(err)
	}
	if second != "" {
		t.Fatalf("duplicate alarm injected: %q", second)
	}
	if HardLimitExceeded(store.Run{Rounds: HardRounds}, store.NowMs()) == false {
		t.Fatal("hard rounds")
	}
}
