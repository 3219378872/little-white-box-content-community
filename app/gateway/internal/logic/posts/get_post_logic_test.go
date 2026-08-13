package posts

import (
	"context"
	"testing"

	"esx/app/content/rpc/contentservice"
	contentpb "esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"gateway/internal/svc"
	"gateway/internal/types"

	"google.golang.org/grpc"
)

type fakeGetPostContentService struct {
	contentservice.ContentService
	getPostFn func(ctx context.Context, in *contentservice.GetPostReq, opts ...grpc.CallOption) (*contentservice.GetPostResp, error)
}

func (f *fakeGetPostContentService) GetPost(ctx context.Context, in *contentservice.GetPostReq, opts ...grpc.CallOption) (*contentservice.GetPostResp, error) {
	return f.getPostFn(ctx, in, opts...)
}

func TestGetPost_ReturnsStatusAndRevision(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		ContentService: &fakeGetPostContentService{
			getPostFn: func(_ context.Context, in *contentservice.GetPostReq, _ ...grpc.CallOption) (*contentservice.GetPostResp, error) {
				if in.PostId != 11 {
					t.Fatalf("unexpected post id %d", in.PostId)
				}
				return &contentservice.GetPostResp{Post: &contentpb.PostInfo{
					Id: 11, AuthorId: 7, Title: "draft", Content: "body", Status: 0, Revision: 4,
				}}, nil
			},
		},
	}
	logic := NewGetPostLogic(context.Background(), svcCtx)
	resp, err := logic.GetPost(&types.GetPostReq{PostId: 11})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != 0 || resp.Revision != 4 || resp.Id != 11 {
		t.Fatalf("expected status/revision from content, got %+v", resp)
	}
}
