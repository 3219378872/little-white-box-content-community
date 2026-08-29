package memory

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"esx/pkg/errx"
)

type blockingScanner struct{}

func (blockingScanner) Check(_ context.Context, text string) error {
	if strings.Contains(strings.ToLower(text), "password") {
		return errx.New(errx.ParamError, "memory content failed threat scan")
	}
	return nil
}

func TestMemoryCapacityScanUndoAndVersion(t *testing.T) {
	store := NewMapStore()
	store.Scanner = blockingScanner{}
	ctx := context.Background()

	entry, changeID, err := store.Add(ctx, 7, TargetMemory, "喜欢科幻电影", "r1", 1)
	if err != nil || entry.ID == 0 || changeID == 0 {
		t.Fatalf("add: entry=%+v change=%d err=%v", entry, changeID, err)
	}
	dup, dupChange, err := store.Add(ctx, 7, TargetMemory, "喜欢科幻电影", "r2", 2)
	if err != nil || dup.ID != entry.ID || dupChange != 0 {
		t.Fatalf("dedup failed: %+v change=%d err=%v", dup, dupChange, err)
	}
	if _, _, err := store.Add(ctx, 7, TargetMemory, "my password is 123", "r3", 3); err == nil {
		t.Fatal("expected threat scan failure")
	}
	over := strings.Repeat("字", CapacityMemory)
	if _, _, err := store.Add(ctx, 7, TargetMemory, over, "r4", 4); err == nil {
		t.Fatal("expected capacity failure")
	}
	if utf8.RuneCountInString(over) != CapacityMemory {
		t.Fatal("fixture")
	}

	replaced, _, err := store.Replace(ctx, 7, entry.ID, "改成纪录电影", entry.Version, "r5", 5)
	if err != nil || replaced.Version != 2 {
		t.Fatalf("replace: %+v err=%v", replaced, err)
	}
	if _, _, err := store.Replace(ctx, 7, entry.ID, "stale", entry.Version, "r6", 6); !errx.Is(err, errx.ContentVersionConflict) {
		t.Fatalf("want version conflict, got %v", err)
	}

	undone, err := store.Undo(ctx, 7, changeID, 7)
	if err == nil {
		t.Fatalf("undo of superseded add should conflict, got %+v", undone)
	}
	list, caps, err := store.List(ctx, 7, TargetMemory)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v caps=%v err=%v", list, caps, err)
	}
	latestChange := int64(0)
	_, ids, err := store.Batch(ctx, 7, "r7", []Op{{Op: OpRemove, ID: replaced.ID, Version: replaced.Version}}, 8)
	if err != nil || len(ids) != 1 {
		t.Fatalf("remove: %v %v", ids, err)
	}
	latestChange = ids[0]
	restored, err := store.Undo(ctx, 7, latestChange, 9)
	if err != nil || restored == nil || restored.Deleted {
		t.Fatalf("undo remove: %+v err=%v", restored, err)
	}
	active, err := store.Active(ctx, 7)
	if err != nil || len(active) != 1 {
		t.Fatalf("active after undo: %v err=%v", active, err)
	}
}
