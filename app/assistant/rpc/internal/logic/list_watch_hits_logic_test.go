package logic

import (
	"context"
	"errors"
	"testing"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/app/assistant/watch"
	"esx/app/content/rpc/contentservice"
	"esx/pkg/errx"

	"google.golang.org/grpc"
)

type fakeListContent struct {
	contentservice.ContentService
	posts []*contentservice.PostInfo
	err   error
}

func (f fakeListContent) GetPostsByIds(_ context.Context, req *contentservice.GetPostsByIdsReq, _ ...grpc.CallOption) (*contentservice.GetPostsByIdsResp, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &contentservice.GetPostsByIdsResp{Posts: f.posts}, nil
}

func TestListWatchHitsFailsClosedWhenVisibilityUnavailable(t *testing.T) {
	for name, content := range map[string]contentservice.ContentService{
		"missing": nil,
		"error":   fakeListContent{err: errors.New("content down")},
	} {
		t.Run(name, func(t *testing.T) {
			store := watch.NewMapStore()
			if err := store.RecordHit(t.Context(), watch.Hit{
				UserID: 2, TaskID: 1, PostID: 11, Title: "secret", Summary: "stale",
			}, "event"); err != nil {
				t.Fatal(err)
			}
			logic := NewListWatchHitsLogic(t.Context(), &svc.ServiceContext{Watch: store, ContentService: content})
			resp, err := logic.ListWatchHits(&pb.ListWatchHitsReq{UserId: 2})
			if err != nil {
				t.Fatal(err)
			}
			if len(resp.Hits) != 1 || resp.Hits[0].Title != "" || resp.Hits[0].Summary != "" {
				t.Fatalf("hits=%+v", resp.Hits)
			}
		})
	}
}

func TestListWatchHitsRedactsUnpublished(t *testing.T) {
	store := watch.NewMapStore()
	if err := store.RecordHit(context.Background(), watch.Hit{
		UserID: 2, TaskID: 1, PostID: 11, Title: "visible", Summary: "ok",
	}, "a"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordHit(context.Background(), watch.Hit{
		UserID: 2, TaskID: 1, PostID: 12, Title: "secret", Summary: "draft",
	}, "b"); err != nil {
		t.Fatal(err)
	}
	logic := NewListWatchHitsLogic(context.Background(), &svc.ServiceContext{
		Watch: store,
		ContentService: fakeListContent{posts: []*contentservice.PostInfo{
			{Id: 11, Title: "visible", Status: 1},
			{Id: 12, Title: "secret", Status: 0},
		}},
	})
	resp, err := logic.ListWatchHits(&pb.ListWatchHitsReq{UserId: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Hits) != 2 {
		t.Fatalf("%+v", resp.Hits)
	}
	byPost := map[int64]*pb.WatchHit{}
	for _, hit := range resp.Hits {
		byPost[hit.PostId] = hit
	}
	if byPost[11].Title == "" || byPost[12].Title != "" || byPost[12].Summary != "" {
		t.Fatalf("%+v", resp.Hits)
	}
}

func TestListWatchHitsNilStoreUnavailable(t *testing.T) {
	logic := NewListWatchHitsLogic(context.Background(), &svc.ServiceContext{})
	_, err := logic.ListWatchHits(&pb.ListWatchHitsReq{UserId: 2})
	if !errx.Is(err, errx.ServiceUnavailable) {
		t.Fatalf("got %v", err)
	}
}
