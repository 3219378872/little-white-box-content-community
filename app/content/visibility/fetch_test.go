package visibility

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/kitex/client/callopt"

	"esx/app/content/rpc/contentservice"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"
)

type fakePosts struct {
	err   error
	resp  *contentservice.GetPostsByIdsResp
	calls int
	last  *contentservice.GetPostsByIdsReq
}

func (f *fakePosts) GetPostsByIds(_ context.Context, in *contentservice.GetPostsByIdsReq, _ ...callopt.Option) (*contentservice.GetPostsByIdsResp, error) {
	f.calls++
	f.last = in
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

func TestPublishedByIDsWithoutTagsRequestsSkipTags(t *testing.T) {
	t.Parallel()
	client := &fakePosts{resp: &contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{
		{Id: 1, Status: visibilityx.PublishedStatus},
	}}}
	got, err := PublishedByIDsWithoutTags(context.Background(), client, []int64{1})
	if err != nil || len(got) != 1 {
		t.Fatalf("expected published post, got %#v err=%v", got, err)
	}
	if client.last == nil || !client.last.SkipTags {
		t.Fatalf("expected SkipTags request, got %#v", client.last)
	}
	if _, err := PublishedByIDs(context.Background(), client, []int64{1}); err != nil || client.last.SkipTags {
		t.Fatalf("PublishedByIDs must keep tags, got %#v err=%v", client.last, err)
	}
}

func TestPublishedByIDsWithoutTagsFailsClosed(t *testing.T) {
	t.Parallel()
	want := errors.New("down")
	if _, err := PublishedByIDsWithoutTags(context.Background(), &fakePosts{err: want}, []int64{1}); !errors.Is(err, want) {
		t.Fatalf("expected rpc error, got %v", err)
	}
}
