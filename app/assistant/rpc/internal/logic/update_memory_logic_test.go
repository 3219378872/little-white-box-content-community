package logic

import (
	"testing"
	"time"

	"esx/app/assistant/rpc/internal/memory"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
)

func TestUpdateMemoryPreservesOmittedFields(t *testing.T) {
	store := memory.NewMapStore()
	if err := store.Apply(t.Context(), 2, memory.Candidate{
		Layer: memory.LayerProfile, Dimension: "topic", Value: "go", Score: 0.7,
		Source: memory.SourceExplicit, Confidence: 1,
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	items, _ := store.List(t.Context(), 2, memory.LayerProfile, time.Now())
	suppressed := true
	logic := NewUpdateMemoryLogic(t.Context(), &svc.ServiceContext{Memory: store})
	if _, err := logic.UpdateMemory(&pb.UpdateMemoryReq{
		UserId: 2, Id: items[0].ID, Suppressed: &suppressed,
	}); err != nil {
		t.Fatal(err)
	}
	updated, _ := store.List(t.Context(), 2, memory.LayerProfile, time.Now())
	if len(updated) != 1 || updated[0].Value != "go" || updated[0].Score != 0.7 || !updated[0].Suppressed {
		t.Fatalf("updated=%+v", updated)
	}
}
