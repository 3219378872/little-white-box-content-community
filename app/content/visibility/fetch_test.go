package visibility

import (
	"context"
	"errors"
	"testing"

	"esx/app/content/rpc/contentservice"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"

	"google.golang.org/grpc"
)

type fakePosts struct {
	err   error
	resp  *contentservice.GetPostsByIdsResp
	calls int
}

func (f *fakePosts) GetPostsByIds(context.Context, *contentservice.GetPostsByIdsReq, ...grpc.CallOption) (*contentservice.GetPostsByIdsResp, error) {
	f.calls++
	return f.resp, f.err
}

func TestPublishedByIDs(t *testing.T) {
	t.Parallel()

	t.Run("nil client fails closed", func(t *testing.T) {
		t.Parallel()
		_, err := PublishedByIDs(context.Background(), nil, []int64{1})
		if !errx.Is(err, errx.ServiceUnavailable) {
			t.Fatalf("expected unavailable, got %v", err)
		}
	})

	t.Run("rpc error fails closed", func(t *testing.T) {
		t.Parallel()
		want := errors.New("down")
		_, err := PublishedByIDs(context.Background(), &fakePosts{err: want}, []int64{1})
		if !errors.Is(err, want) {
			t.Fatalf("expected rpc error, got %v", err)
		}
	})

	t.Run("keeps published only", func(t *testing.T) {
		t.Parallel()
		client := &fakePosts{resp: &contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{
			{Id: 1, Status: visibilityx.PublishedStatus, Title: "live"},
			{Id: 2, Status: visibilityx.DraftStatus, Title: "draft"},
		}}}
		got, err := PublishedByIDs(context.Background(), client, []int64{1, 2})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(got) != 1 || got[1].Title != "live" {
			t.Fatalf("expected published post 1, got %#v", got)
		}
	})
}
