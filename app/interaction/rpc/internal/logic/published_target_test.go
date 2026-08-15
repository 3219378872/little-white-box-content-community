package logic

import (
	"context"
	"testing"

	"errx"
	"esx/app/content/rpc/contentservice"
	contentpb "esx/app/content/rpc/pb/xiaobaihe/content/pb"

	"google.golang.org/grpc"
)

type fakeContentService struct {
	getPost func(ctx context.Context, in *contentservice.GetPostReq) (*contentservice.GetPostResp, error)
}

func (f *fakeContentService) GetPost(ctx context.Context, in *contentservice.GetPostReq, _ ...grpc.CallOption) (*contentservice.GetPostResp, error) {
	if f.getPost == nil {
		return &contentservice.GetPostResp{Post: &contentpb.PostInfo{Id: in.PostId, Status: 1}}, nil
	}
	return f.getPost(ctx, in)
}

func TestRequirePublishedPost_MissingContentFailsClosed(t *testing.T) {
	err := requirePublishedPost(context.Background(), nil, 11)
	if !errx.Is(err, errx.ServiceUnavailable) {
		t.Fatalf("got %v, want ServiceUnavailable", err)
	}
}

func TestRequirePublishedPost_DraftIsNotFound(t *testing.T) {
	content := &fakeContentService{getPost: func(_ context.Context, in *contentservice.GetPostReq) (*contentservice.GetPostResp, error) {
		return &contentservice.GetPostResp{Post: &contentpb.PostInfo{Id: in.PostId, Status: 0}}, nil
	}}
	err := requirePublishedPost(context.Background(), content, 11)
	if !errx.Is(err, errx.ContentNotFound) {
		t.Fatalf("got %v, want ContentNotFound", err)
	}
}

func TestRequirePublishedPost_PublishedOK(t *testing.T) {
	if err := requirePublishedPost(context.Background(), &fakeContentService{}, 11); err != nil {
		t.Fatal(err)
	}
}
