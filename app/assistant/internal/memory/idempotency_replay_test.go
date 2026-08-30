package memory

import (
	"context"
	"testing"
)

func TestMapStoreReplaysCommittedReplaceAndRemoveByRequestID(t *testing.T) {
	ctx := context.Background()
	mem := NewMapStore()
	entry, _, err := mem.Add(ctx, 1, TargetMemory, "before", "add-request", 1)
	if err != nil {
		t.Fatal(err)
	}
	replaced, firstChange, err := mem.Replace(ctx, 1, entry.ID, "after", entry.Version, "replace-request", 2)
	if err != nil {
		t.Fatal(err)
	}
	replayed, secondChange, err := mem.Replace(ctx, 1, entry.ID, "after", entry.Version, "replace-request", 3)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Version != replaced.Version || replayed.Content != replaced.Content || secondChange != firstChange {
		t.Fatalf("replace replay=%+v/%d first=%+v/%d", replayed, secondChange, replaced, firstChange)
	}
	firstRemove, err := mem.Remove(ctx, 1, entry.ID, replaced.Version, "remove-request", 4)
	if err != nil {
		t.Fatal(err)
	}
	secondRemove, err := mem.Remove(ctx, 1, entry.ID, replaced.Version, "remove-request", 5)
	if err != nil || secondRemove != firstRemove {
		t.Fatalf("remove replay=%d first=%d err=%v", secondRemove, firstRemove, err)
	}
}
