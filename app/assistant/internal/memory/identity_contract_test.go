package memory

import (
	"context"
	"esx/pkg/errx"
	"strings"
	"sync"
	"testing"
)

func TestMapStoreMutationIdentityContract(t *testing.T) {
	for _, target := range []string{TargetMemory, TargetUser} {
		t.Run(target, func(t *testing.T) { memoryMutationContract(t, NewMapStore(), target) })
	}
}

func memoryMutationContract(t *testing.T, st Store, target string) {
	t.Helper()
	ctx := context.Background()
	first, _, err := st.Add(ctx, 10, target, "first", "first", 1)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := st.Add(ctx, 10, target, "second", "second", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.Replace(ctx, 10, second.ID, " FIRST ", second.Version, "dup-replace", 2); !errx.Is(err, errx.ParamError) {
		t.Fatalf("duplicate replace %v", err)
	}
	removed, err := st.Remove(ctx, 10, first.ID, first.Version, "remove", 3)
	if err != nil {
		t.Fatal(err)
	}
	restored, _, err := st.Add(ctx, 10, target, "first", "add-again", 4)
	if err != nil || restored.ID == first.ID {
		t.Fatalf("deleted identity reused: %+v %v", restored, err)
	}
	if _, err := st.Undo(ctx, 10, removed, 5); !errx.Is(err, errx.ContentVersionConflict) {
		t.Fatalf("duplicate undo %v", err)
	}
	ops := []Op{{Op: OpAdd, Target: target, Content: "batch one"}, {Op: OpAdd, Target: target, Content: "batch two"}}
	entries, changes, err := st.Batch(ctx, 11, "batch", ops, 6)
	if err != nil || len(entries) != 2 || len(changes) != 2 {
		t.Fatalf("batch=%+v changes=%v err=%v", entries, changes, err)
	}
	replay, replayChanges, err := st.Batch(ctx, 11, "batch", ops, 7)
	if err != nil || len(replay) != 2 || replay[1].ID != entries[1].ID || len(replayChanges) != 2 || replayChanges[1] != changes[1] {
		t.Fatalf("batch replay=%+v changes=%v err=%v", replay, replayChanges, err)
	}
	_, _, err = st.Batch(ctx, 12, "failed", []Op{{Op: OpAdd, Target: target, Content: "must roll back"}, {Op: OpAdd, Target: target, Content: strings.Repeat("字", LimitFor(target))}}, 8)
	active, _ := st.Active(ctx, 12)
	if err == nil || len(active) != 0 {
		t.Fatalf("batch rollback active=%+v err=%v", active, err)
	}
	if _, _, err := st.Batch(ctx, 13, "empty", nil, 8); !errx.Is(err, errx.ParamError) {
		t.Fatalf("empty batch %v", err)
	}
	// Serialize exact duplicates and preserve distinct common-prefix additions.
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	start := make(chan struct{})
	for i, text := range []string{strings.Repeat("甲", 512) + "一", strings.Repeat("甲", 512) + "二", strings.Repeat("甲", 512) + "一", strings.Repeat("甲", 512) + "二"} {
		wg.Add(1)
		go func(i int, text string) {
			defer wg.Done()
			<-start
			_, _, err := st.Add(ctx, 14, target, text, itoa(int64(i)), 9)
			errs <- err
		}(i, text)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	active, err = st.Active(ctx, 14)
	if err != nil || len(active) != 2 {
		t.Fatalf("concurrent entries=%+v err=%v", active, err)
	}
}
