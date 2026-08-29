package logic

import (
	"testing"

	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/pkg/errx"
)

func TestListMemoryReturnsStoredItems(t *testing.T) {
	mem := memory.NewMapStore()
	if _, _, err := mem.Add(t.Context(), 2, memory.TargetMemory, "剧情向", "r1", store.NowMs()); err != nil {
		t.Fatal(err)
	}
	logic := NewListMemoryLogic(t.Context(), &svc.ServiceContext{Memory: mem})
	resp, err := logic.ListMemory(&pb.ListMemoryReq{UserId: 2, Target: memory.TargetMemory})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 1 || resp.Items[0].Content != "剧情向" {
		t.Fatalf("%+v", resp.Items)
	}
}

func TestListMemoryNilStoreUnavailable(t *testing.T) {
	logic := NewListMemoryLogic(t.Context(), &svc.ServiceContext{})
	_, err := logic.ListMemory(&pb.ListMemoryReq{UserId: 2})
	if !errx.Is(err, errx.ServiceUnavailable) {
		t.Fatalf("got %v", err)
	}
}
