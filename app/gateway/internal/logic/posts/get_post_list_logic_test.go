package posts

import (
	"context"
	"testing"

	"esx/app/content/rpc/contentservice"
	contentpb "esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	userpb "esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/app/user/rpc/userservice"
	"esx/pkg/jwtx"

	"google.golang.org/grpc"
)

type fakePostListContentService struct {
	contentservice.ContentService
	getPostListFn func(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error)
}

func (f *fakePostListContentService) GetPostList(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
	return f.getPostListFn(ctx, in, opts...)
}

func newLogic(t *testing.T, ctx context.Context, expectedUserId int64) *GetPostListLogic {
	t.Helper()

	svcCtx := &svc.ServiceContext{
		ContentService: &fakePostListContentService{
			getPostListFn: func(_ context.Context, in *contentservice.GetPostListReq, _ ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
				if in.Cursor != "" || in.PageSize != 20 || in.SortBy != 1 {
					t.Fatalf("unexpected rpc req: %+v", in)
				}
				if in.UserId != expectedUserId {
					t.Fatalf("expected UserId %d, got %d", expectedUserId, in.UserId)
				}
				return &contentservice.GetPostListResp{
					Posts: []*contentpb.PostInfo{
						{
							Id:            100,
							AuthorId:      200,
							Title:         "title",
							Content:       "content",
							Images:        []string{"img"},
							Tags:          []string{"tag"},
							ViewCount:     10,
							LikeCount:     20,
							CommentCount:  30,
							FavoriteCount: 40,
							CreatedAt:     50,
						},
					},
					NextCursor: "",
				}, nil
			},
		},
	}

	return NewGetPostListLogic(ctx, svcCtx)
}

func TestGetPostList_AnonymousContext_ReturnsPublicData(t *testing.T) {
	l := newLogic(t, context.Background(), 0)

	resp, err := l.GetPostList(&types.GetPostListReq{
		PageSize: 20,
		SortBy:   1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.NextCursor != "" || len(resp.List) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.List[0].Id != 100 || resp.List[0].AuthorId != 200 {
		t.Fatalf("unexpected item: %+v", resp.List[0])
	}
	if resp.List[0].FavoriteCount != 40 {
		t.Fatalf("expected FavoriteCount from content, got %+v", resp.List[0])
	}
}

func TestGetPostList_AuthenticatedContext_ReturnsSamePublicData(t *testing.T) {
	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{
		UserId:   42,
		Username: "alice",
	})
	l := newLogic(t, ctx, 42)

	resp, err := l.GetPostList(&types.GetPostListReq{
		PageSize: 20,
		SortBy:   1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.NextCursor != "" || len(resp.List) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.List[0].Id != 100 || resp.List[0].AuthorId != 200 {
		t.Fatalf("unexpected item: %+v", resp.List[0])
	}
	if resp.List[0].FavoriteCount != 40 {
		t.Fatalf("expected FavoriteCount from content, got %+v", resp.List[0])
	}
}

func TestGetPostList_EnrichesAuthor(t *testing.T) {
	l := newLogic(t, context.Background(), 0)
	l.svcCtx.UserService = &fakeGetPostUserService{
		batchGetUsersFn: func(_ context.Context, in *userservice.BatchGetUsersReq, _ ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
			if len(in.UserIds) != 1 || in.UserIds[0] != 200 {
				t.Fatalf("unexpected author ids %v", in.UserIds)
			}
			return &userservice.BatchGetUsersResp{Users: []*userpb.UserInfo{
				{Id: 200, Nickname: "Alice", AvatarUrl: " https://a.png "},
			}}, nil
		},
	}
	resp, err := l.GetPostList(&types.GetPostListReq{PageSize: 20, SortBy: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.List[0].AuthorName != "Alice" || resp.List[0].AuthorAvatar != "https://a.png" {
		t.Fatalf("expected author enrichment, got %+v", resp.List[0])
	}
}
