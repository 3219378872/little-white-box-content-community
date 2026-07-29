package rebuild

import (
	"context"
	"errors"
	"testing"

	"esx/app/content/rpc/contentservice"
	"esx/app/search/mq/internal/indexer"

	"google.golang.org/grpc"
)

type fakeSource struct {
	pages map[int32]*contentservice.GetPostListResp
	err   error
}

func (f *fakeSource) GetPostList(_ context.Context, req *contentservice.GetPostListReq, _ ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.pages[req.Page], nil
}

type fakeTarget struct {
	docs       []indexer.IndexDoc
	indexErrAt int
	refreshed  bool
}

func (f *fakeTarget) Index(_ context.Context, doc indexer.IndexDoc) error {
	if f.indexErrAt > 0 && len(f.docs)+1 == f.indexErrAt {
		return errors.New("index unavailable")
	}
	f.docs = append(f.docs, doc)
	return nil
}

func (f *fakeTarget) Refresh(context.Context) error {
	f.refreshed = true
	return nil
}

func TestRunIndexesPublishedPostsAcrossPages(t *testing.T) {
	source := &fakeSource{pages: map[int32]*contentservice.GetPostListResp{
		1: {Total: 3, Posts: []*contentservice.PostInfo{
			{Id: 1, AuthorId: 10, Title: "one", Content: "body", Tags: []string{"go"}, Status: 1, LikeCount: 4},
			{Id: 2, Status: 0},
		}},
		2: {Total: 3, Posts: []*contentservice.PostInfo{{Id: 3, Status: 1, CommentCount: 2}}},
	}}
	target := &fakeTarget{}

	count, err := Run(context.Background(), source, target, 2)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || len(target.docs) != 2 || !target.refreshed {
		t.Fatalf("count=%d docs=%d refreshed=%v", count, len(target.docs), target.refreshed)
	}
	if got := target.docs[0].Body["like_count"]; got != int64(4) {
		t.Fatalf("like_count=%v", got)
	}
}

func TestRunStopsOnIndexFailureWithoutRefresh(t *testing.T) {
	source := &fakeSource{pages: map[int32]*contentservice.GetPostListResp{
		1: {Total: 1, Posts: []*contentservice.PostInfo{{Id: 1, Status: 1}}},
	}}
	target := &fakeTarget{indexErrAt: 1}

	count, err := Run(context.Background(), source, target, 1)
	if err == nil || count != 0 || target.refreshed {
		t.Fatalf("count=%d err=%v refreshed=%v", count, err, target.refreshed)
	}
}

func TestRunRejectsInvalidConfiguration(t *testing.T) {
	if _, err := Run(context.Background(), nil, &fakeTarget{}, 1); err == nil {
		t.Fatal("expected missing source error")
	}
	if _, err := Run(context.Background(), &fakeSource{}, &fakeTarget{}, MaxPageSize+1); err == nil {
		t.Fatal("expected page size error")
	}
}
