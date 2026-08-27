package logic

import (
	"testing"
	"time"

	"esx/app/assistant/rpc/internal/memory"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
)

func TestListMemoryReturnsStoredItems(t *testing.T) {
	store := memory.NewMapStore()
	if err := store.Apply(t.Context(), 2, memory.Candidate{
		Layer: memory.LayerProfile, Dimension: "topic", Value: "剧情", Score: 0.9,
		Source: memory.SourceExplicit, Confidence: 1,
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	logic := NewListMemoryLogic(t.Context(), &svc.ServiceContext{Memory: store})
	resp, err := logic.ListMemory(&pb.ListMemoryReq{UserId: 2, Layer: memory.LayerProfile})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 1 || resp.Items[0].Value != "剧情" || !resp.Items[0].Confirmed {
		t.Fatalf("%+v", resp.Items)
	}
}
