package user

import (
	"context"
	"testing"

	"esx/app/content/rpc/contentservice"
	contentpb "esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	userpb "esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/app/user/rpc/userservice"

	"google.golang.org/grpc"
)

type fakeContentServiceUserPosts struct {
	contentservice.ContentService
	getUserPostsFn func(ctx context.Context, in *contentservice.GetUserPostsReq, opts ...grpc.CallOption) (*contentservice.GetUserPostsResp, error)
}

func (f *fakeContentServiceUserPosts) GetUserPosts(ctx context.Context, in *contentservice.GetUserPostsReq, opts ...grpc.CallOption) (*contentservice.GetUserPostsResp, error) {
	return f.getUserPostsFn(ctx, in, opts...)
}

func TestGetUserPosts_EnrichesAuthorInfo(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		UserService: &fakeUserService{
			batchGetUsersFn: func(_ context.Context, in *userservice.BatchGetUsersReq, _ ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
				if len(in.UserIds) != 2 || in.UserIds[0] != 7 || in.UserIds[1] != 9 {
					t.Fatalf("expected deduped author ids [7 9], got %v", in.UserIds)
				}
				return &userservice.BatchGetUsersResp{Users: []*userpb.UserInfo{
					{Id: 7, Nickname: "Alice", AvatarUrl: " https://media/alice.png "},
					{Id: 9, Username: "bob", AvatarUrl: "https://media/bob.png"},
				}}, nil
			},
		},
		ContentService: &fakeContentServiceUserPosts{
			getUserPostsFn: func(_ context.Context, in *contentservice.GetUserPostsReq, _ ...grpc.CallOption) (*contentservice.GetUserPostsResp, error) {
				if in.UserId != 42 || in.Cursor != "" || in.PageSize != 20 {
					t.Fatalf("unexpected rpc req: %+v", in)
				}
				return &contentservice.GetUserPostsResp{
					Posts: []*contentpb.PostInfo{
						{Id: 100, AuthorId: 7, Title: "A", Status: 1},
						{Id: 200, AuthorId: 9, Title: "B", Status: 1},
						{Id: 300, AuthorId: 7, Title: "C", Status: 1},
					},
					NextCursor: "",
				}, nil
			},
		},
	}

	l := NewGetUserPostsLogic(context.Background(), svcCtx)
	resp, err := l.GetUserPosts(&types.GetUserPostsReq{UserId: 42, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.List) != 3 {
		t.Fatalf("expected 3 items, got %d", len(resp.List))
	}
	if resp.List[0].AuthorId != 7 || resp.List[0].AuthorName != "Alice" || resp.List[0].AuthorAvatar != "https://media/alice.png" {
		t.Fatalf("expected post 100 author enriched, got %+v", resp.List[0])
	}
	if resp.List[1].AuthorName != "bob" || resp.List[1].AuthorAvatar != "https://media/bob.png" {
		t.Fatalf("expected empty nickname to fall back to username, got %+v", resp.List[1])
	}
	if resp.List[2].AuthorName != "Alice" {
		t.Fatalf("expected repeated author to be enriched too, got %+v", resp.List[2])
	}
}

func TestGetUserPosts_BatchGetUsersError_DegradesToEmptyAuthorFields(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		UserService: &fakeUserService{
			batchGetUsersFn: func(_ context.Context, _ *userservice.BatchGetUsersReq, _ ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
				return nil, context.DeadlineExceeded
			},
		},
		ContentService: &fakeContentServiceUserPosts{
			getUserPostsFn: func(_ context.Context, _ *contentservice.GetUserPostsReq, _ ...grpc.CallOption) (*contentservice.GetUserPostsResp, error) {
				return &contentservice.GetUserPostsResp{
					Posts:      []*contentpb.PostInfo{{Id: 100, AuthorId: 7, Title: "A", Status: 1}},
					NextCursor: "",
				}, nil
			},
		},
	}

	l := NewGetUserPostsLogic(context.Background(), svcCtx)
	resp, err := l.GetUserPosts(&types.GetUserPostsReq{UserId: 42, PageSize: 20})
	if err != nil {
		t.Fatalf("expected list to survive author lookup failure, got %v", err)
	}
	if len(resp.List) != 1 || resp.List[0].AuthorName != "" || resp.List[0].AuthorAvatar != "" {
		t.Fatalf("expected empty author fields on BatchGetUsers failure, got %+v", resp.List)
	}
}
