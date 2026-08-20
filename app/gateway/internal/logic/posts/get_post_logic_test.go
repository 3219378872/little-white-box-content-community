package posts

import (
	"context"
	"testing"

	"esx/app/content/rpc/contentservice"
	contentpb "esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/app/interaction/rpc/interactionservice"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"
	"user/userservice"

	"google.golang.org/grpc"
)

type fakeGetPostInteractionService struct {
	interactionservice.InteractionService
	liked     map[int64]bool
	favorited map[int64]bool
}

func (f *fakeGetPostInteractionService) BatchCheckLiked(_ context.Context, in *interactionservice.BatchCheckLikedReq, _ ...grpc.CallOption) (*interactionservice.BatchCheckLikedResp, error) {
	results := make(map[int64]bool, len(in.TargetIds))
	for _, id := range in.TargetIds {
		results[id] = f.liked[id]
	}
	return &interactionservice.BatchCheckLikedResp{Results: results}, nil
}

func (f *fakeGetPostInteractionService) BatchCheckFavorited(_ context.Context, in *interactionservice.BatchCheckFavoritedReq, _ ...grpc.CallOption) (*interactionservice.BatchCheckFavoritedResp, error) {
	results := make(map[int64]bool, len(in.PostIds))
	for _, id := range in.PostIds {
		results[id] = f.favorited[id]
	}
	return &interactionservice.BatchCheckFavoritedResp{Results: results}, nil
}

type fakeGetPostContentService struct {
	contentservice.ContentService
	getPostFn func(ctx context.Context, in *contentservice.GetPostReq, opts ...grpc.CallOption) (*contentservice.GetPostResp, error)
}

func (f *fakeGetPostContentService) GetPost(ctx context.Context, in *contentservice.GetPostReq, opts ...grpc.CallOption) (*contentservice.GetPostResp, error) {
	return f.getPostFn(ctx, in, opts...)
}

type fakeGetPostUserService struct {
	userservice.UserService
	batchGetUsersFn func(context.Context, *userservice.BatchGetUsersReq, ...grpc.CallOption) (*userservice.BatchGetUsersResp, error)
}

func (f *fakeGetPostUserService) BatchGetUsers(ctx context.Context, in *userservice.BatchGetUsersReq, opts ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
	return f.batchGetUsersFn(ctx, in, opts...)
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
	if resp.IsLiked || resp.IsFavorited {
		t.Fatalf("anonymous viewer must not receive interaction state, got %+v", resp)
	}
}

func TestGetPost_AuthenticatedFillsViewerState(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		ContentService: &fakeGetPostContentService{
			getPostFn: func(_ context.Context, _ *contentservice.GetPostReq, _ ...grpc.CallOption) (*contentservice.GetPostResp, error) {
				return &contentservice.GetPostResp{Post: &contentpb.PostInfo{
					Id: 11, AuthorId: 7, Title: "pub", Content: "body", Status: 1, Revision: 2,
				}}, nil
			},
		},
		InteractionService: &fakeGetPostInteractionService{
			liked:     map[int64]bool{11: true},
			favorited: map[int64]bool{11: false},
		},
	}
	ctx := jwtx.WithUserIdContext(context.Background(), 42)
	logic := NewGetPostLogic(ctx, svcCtx)
	resp, err := logic.GetPost(&types.GetPostReq{PostId: 11})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsLiked || resp.IsFavorited {
		t.Fatalf("expected liked=true favorited=false, got %+v", resp)
	}
}

func TestGetPost_HydratesAuthorProfile(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		ContentService: &fakeGetPostContentService{
			getPostFn: func(_ context.Context, _ *contentservice.GetPostReq, _ ...grpc.CallOption) (*contentservice.GetPostResp, error) {
				return &contentservice.GetPostResp{Post: &contentpb.PostInfo{
					Id: 11, AuthorId: 7, Title: "pub", Content: "body", Status: 1, Revision: 2,
				}}, nil
			},
		},
		UserService: &fakeGetPostUserService{
			batchGetUsersFn: func(_ context.Context, in *userservice.BatchGetUsersReq, _ ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
				if len(in.UserIds) != 1 || in.UserIds[0] != 7 {
					t.Fatalf("unexpected author ids %+v", in.UserIds)
				}
				return &userservice.BatchGetUsersResp{Users: []*userservice.UserInfo{{
					Id: 7, Nickname: "Alice", Username: "alice", AvatarUrl: "https://avatar/7.png",
				}}}, nil
			},
		},
	}
	resp, err := NewGetPostLogic(context.Background(), svcCtx).GetPost(&types.GetPostReq{PostId: 11})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AuthorId != 7 || resp.AuthorName != "Alice" || resp.AuthorAvatar != "https://avatar/7.png" {
		t.Fatalf("expected hydrated author, got %+v", resp)
	}
}
