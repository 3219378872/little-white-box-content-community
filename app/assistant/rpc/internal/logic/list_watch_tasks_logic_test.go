package logic

import (
	"context"
	"testing"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/pkg/errx"
)

func TestListWatchTasksNilStoreUnavailable(t *testing.T) {
	logic := NewListWatchTasksLogic(context.Background(), &svc.ServiceContext{})
	_, err := logic.ListWatchTasks(&pb.ListWatchTasksReq{UserId: 2})
	if !errx.Is(err, errx.ServiceUnavailable) {
		t.Fatalf("got %v", err)
	}
}
