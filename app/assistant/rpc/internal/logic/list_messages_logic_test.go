package logic

import (
	"context"
	"testing"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/pkg/errx"
)

func TestListMessagesReturnsLatestPageAndOlderCursor(t *testing.T) {
	mem := store.NewMemoryStore()
	for i := 0; i < 5; i++ {
		if _, err := mem.InsertMessage(context.Background(), store.Message{
			UserID: 7, SessionID: 1, Role: store.RoleUser, Kind: store.KindMessage,
			Content: "message", Visible: true, CreatedAtMs: int64(i + 1),
		}); err != nil {
			t.Fatal(err)
		}
	}
	logic := NewListMessagesLogic(context.Background(), &svc.ServiceContext{Store: mem})
	latest, err := logic.ListMessages(&pb.ListMessagesReq{UserId: 7, SessionId: 1, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(latest.Messages) != 2 || !latest.HasMore || latest.NextBeforeId != latest.Messages[0].Id {
		t.Fatalf("latest=%+v", latest)
	}
	if latest.Messages[0].Id >= latest.Messages[1].Id {
		t.Fatalf("latest page must be ascending: %+v", latest.Messages)
	}
	older, err := logic.ListMessages(&pb.ListMessagesReq{UserId: 7, SessionId: 1, Limit: 2, BeforeId: latest.NextBeforeId})
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Messages) != 2 || older.Messages[1].Id >= latest.Messages[0].Id {
		t.Fatalf("older=%+v latest=%+v", older, latest)
	}
}

func TestListMessagesRejectsBeforeAndAfterTogether(t *testing.T) {
	logic := NewListMessagesLogic(context.Background(), &svc.ServiceContext{Store: store.NewMemoryStore()})
	_, err := logic.ListMessages(&pb.ListMessagesReq{UserId: 7, BeforeId: 10, AfterId: 1})
	if !errx.Is(err, errx.ParamError) {
		t.Fatalf("err=%v", err)
	}
}
