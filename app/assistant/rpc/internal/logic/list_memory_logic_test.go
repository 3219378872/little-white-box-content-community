package logic

import (
	"testing"

	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/pkg/errx"
)

func TestListMemoryRequiresUser(t *testing.T) {
	logic := NewListMemoryLogic(t.Context(), nil)
	if _, err := logic.ListMemory(nil); !errx.Is(err, errx.LoginRequired) {
		t.Fatalf("nil req: got %v", err)
	}
	if _, err := logic.ListMemory(&pb.ListMemoryReq{}); !errx.Is(err, errx.LoginRequired) {
		t.Fatalf("zero user: got %v", err)
	}
	resp, err := logic.ListMemory(&pb.ListMemoryReq{UserId: 2})
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil || resp.Items == nil {
		t.Fatal("expected empty items slice")
	}
}

func TestUpdateMemoryUnavailableUntilStore(t *testing.T) {
	logic := NewUpdateMemoryLogic(t.Context(), nil)
	_, err := logic.UpdateMemory(&pb.UpdateMemoryReq{UserId: 2, Id: 1})
	if !errx.Is(err, errx.ServiceUnavailable) {
		t.Fatalf("got %v", err)
	}
}
