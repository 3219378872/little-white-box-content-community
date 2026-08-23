package user

import (
	"context"
	"testing"

	"esx/app/content/rpc/contentservice"
	contentpb "esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/interaction/rpc/interactionservice"
	interactionpb "esx/app/interaction/rpc/pb/xiaobaihe/interaction/pb"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"google.golang.org/grpc"
)

type fakeUserService struct {
	userservice.UserService
	getUserFn       func(ctx context.Context, in *pb.GetUserReq) (*pb.GetUserResp, error)
	batchGetUsersFn func(ctx context.Context, in *userservice.BatchGetUsersReq, opts ...grpc.CallOption) (*userservice.BatchGetUsersResp, error)
}

func (f *fakeUserService) GetUser(ctx context.Context, in *pb.GetUserReq, _ ...grpc.CallOption) (*pb.GetUserResp, error) {
	return f.getUserFn(ctx, in)
}

func (f *fakeUserService) BatchGetUsers(ctx context.Context, in *userservice.BatchGetUsersReq, opts ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
	if f.batchGetUsersFn == nil {
		return &userservice.BatchGetUsersResp{}, nil
	}
	return f.batchGetUsersFn(ctx, in, opts...)
}

func buildFavoritesLogic(requesterID int64, visibility int32) *GetUserFavoritesLogic {
	svcCtx := &svc.ServiceContext{
		UserService: &fakeUserService{
			getUserFn: func(_ context.Context, in *pb.GetUserReq) (*pb.GetUserResp, error) {
				return &pb.GetUserResp{
					User: &pb.UserInfo{
						Id:                  in.UserId,
						FavoritesVisibility: visibility,
					},
				}, nil
			},
		},
		InteractionService: &fakeInteractionServiceFavorites{
			getFavoriteListFn: func(_ context.Context, _ *interactionpb.GetFavoriteListReq, _ ...grpc.CallOption) (*interactionpb.GetFavoriteListResp, error) {
				return &interactionpb.GetFavoriteListResp{PostIds: []int64{}, Total: 0}, nil
			},
		},
	}
	ctx := context.Background()
	if requesterID != 0 {
		ctx = jwtx.WithUserIdContext(ctx, requesterID)
	}
	return NewGetUserFavoritesLogic(ctx, svcCtx)
}

func TestGetUserFavorites_PrivateAndNotOwner_ReturnsFavoritesPrivate(t *testing.T) {
	l := buildFavoritesLogic(10, 2)
	_, err := l.GetUserFavorites(&types.GetUserFavoritesReq{UserId: 99, Page: 1, PageSize: 20})
	if err == nil {
		t.Fatal("expected FavoritesPrivate error, got nil")
	}
	if !errx.Is(err, errx.FavoritesPrivate) {
		t.Fatalf("expected FavoritesPrivate, got: %v", err)
	}
}

func TestGetUserFavorites_PrivateAndUnauthenticated_ReturnsFavoritesPrivate(t *testing.T) {
	l := buildFavoritesLogic(0, 2)
	_, err := l.GetUserFavorites(&types.GetUserFavoritesReq{UserId: 99, Page: 1, PageSize: 20})
	if !errx.Is(err, errx.FavoritesPrivate) {
		t.Fatalf("expected FavoritesPrivate, got: %v", err)
	}
}

func TestGetUserFavorites_PrivateAndOwner_ReturnsEmptyList(t *testing.T) {
	l := buildFavoritesLogic(42, 2)
	resp, err := l.GetUserFavorites(&types.GetUserFavoritesReq{UserId: 42, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected resp, got nil")
	}
	if len(resp.List) != 0 {
		t.Fatalf("expected empty list, got %d items", len(resp.List))
	}
	if resp.NextCursor != "" {
		t.Fatalf("expected empty NextCursor on empty favorites, got %q", resp.NextCursor)
	}
}

func TestGetUserFavorites_PublicAndNotOwner_ReturnsEmptyList(t *testing.T) {
	l := buildFavoritesLogic(10, 1)
	resp, err := l.GetUserFavorites(&types.GetUserFavoritesReq{UserId: 99, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected resp, got nil")
	}
	if len(resp.List) != 0 {
		t.Fatalf("expected empty list, got %d items", len(resp.List))
	}
}

type fakeInteractionServiceFavorites struct {
	interactionservice.InteractionService
	getFavoriteListFn func(ctx context.Context, in *interactionpb.GetFavoriteListReq, opts ...grpc.CallOption) (*interactionpb.GetFavoriteListResp, error)
	liked             map[int64]bool
	favorited         map[int64]bool
}

func (f *fakeInteractionServiceFavorites) GetFavoriteList(ctx context.Context, in *interactionpb.GetFavoriteListReq, opts ...grpc.CallOption) (*interactionpb.GetFavoriteListResp, error) {
	return f.getFavoriteListFn(ctx, in, opts...)
}

func (f *fakeInteractionServiceFavorites) BatchCheckLiked(_ context.Context, in *interactionpb.BatchCheckLikedReq, _ ...grpc.CallOption) (*interactionpb.BatchCheckLikedResp, error) {
	result := make(map[int64]bool, len(in.TargetIds))
	for _, id := range in.TargetIds {
		if f.liked != nil && f.liked[id] {
			result[id] = true
		}
	}
	return &interactionpb.BatchCheckLikedResp{Results: result}, nil
}

func (f *fakeInteractionServiceFavorites) BatchCheckFavorited(_ context.Context, in *interactionpb.BatchCheckFavoritedReq, _ ...grpc.CallOption) (*interactionpb.BatchCheckFavoritedResp, error) {
	result := make(map[int64]bool, len(in.PostIds))
	for _, id := range in.PostIds {
		if f.favorited != nil && f.favorited[id] {
			result[id] = true
		}
	}
	return &interactionpb.BatchCheckFavoritedResp{Results: result}, nil
}

type fakeContentServiceFavorites struct {
	contentservice.ContentService
	getPostsByIdsFn func(ctx context.Context, in *contentpb.GetPostsByIdsReq, opts ...grpc.CallOption) (*contentpb.GetPostsByIdsResp, error)
}

func (f *fakeContentServiceFavorites) GetPostsByIds(ctx context.Context, in *contentpb.GetPostsByIdsReq, opts ...grpc.CallOption) (*contentpb.GetPostsByIdsResp, error) {
	return f.getPostsByIdsFn(ctx, in, opts...)
}

func TestGetUserFavorites_WithData_ReturnsPosts(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		UserService: &fakeUserService{
			getUserFn: func(_ context.Context, in *pb.GetUserReq) (*pb.GetUserResp, error) {
				return &pb.GetUserResp{User: &pb.UserInfo{Id: in.UserId, FavoritesVisibility: 1}}, nil
			},
			batchGetUsersFn: func(_ context.Context, in *userservice.BatchGetUsersReq, _ ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
				if len(in.UserIds) != 2 || in.UserIds[0] != 1 || in.UserIds[1] != 2 {
					t.Fatalf("expected deduped author ids [1 2], got %v", in.UserIds)
				}
				return &userservice.BatchGetUsersResp{Users: []*pb.UserInfo{
					{Id: 1, Nickname: "Alice", AvatarUrl: " https://media/alice.png "},
					{Id: 2, Username: "bob", AvatarUrl: "https://media/bob.png"},
				}}, nil
			},
		},
		InteractionService: &fakeInteractionServiceFavorites{
			getFavoriteListFn: func(_ context.Context, in *interactionpb.GetFavoriteListReq, _ ...grpc.CallOption) (*interactionpb.GetFavoriteListResp, error) {
				if in.UserId != 42 {
					t.Fatalf("expected userId 42, got %d", in.UserId)
				}
				return &interactionpb.GetFavoriteListResp{
					PostIds: []int64{100, 200},
					Total:   2,
				}, nil
			},
			liked:     map[int64]bool{100: true},
			favorited: map[int64]bool{100: true, 200: true},
		},
		ContentService: &fakeContentServiceFavorites{
			getPostsByIdsFn: func(_ context.Context, in *contentpb.GetPostsByIdsReq, _ ...grpc.CallOption) (*contentpb.GetPostsByIdsResp, error) {
				if len(in.PostIds) != 2 {
					t.Fatalf("expected 2 post ids, got %d", len(in.PostIds))
				}
				return &contentpb.GetPostsByIdsResp{
					Posts: []*contentpb.PostInfo{
						{Id: 100, AuthorId: 1, Title: "Post A", Content: "Content A", Status: 1, ViewCount: 10, LikeCount: 5, CommentCount: 2, FavoriteCount: 3, CreatedAt: 1000},
						{Id: 200, AuthorId: 2, Title: "Post B", Content: "Content B", Status: 1, ViewCount: 20, LikeCount: 10, CommentCount: 4, FavoriteCount: 6, CreatedAt: 2000},
					},
				}, nil
			},
		},
	}

	ctx := jwtx.WithUserIdContext(context.Background(), 42)
	l := NewGetUserFavoritesLogic(ctx, svcCtx)

	resp, err := l.GetUserFavorites(&types.GetUserFavoritesReq{UserId: 42, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.List) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.List))
	}
	if resp.List[0].Id != 100 || resp.List[1].Id != 200 {
		t.Fatalf("unexpected items: %+v", resp.List)
	}
	if resp.NextCursor != "" {
		t.Fatalf("expected no NextCursor for partial batch, got %q", resp.NextCursor)
	}
	// CORE-032：列表应回填当前访问者的互动状态。
	if !resp.List[0].IsLiked || resp.List[0].IsFavorited != true {
		t.Fatalf("expected post 100 liked+favorited, got %+v", resp.List[0])
	}
	if resp.List[1].IsLiked || !resp.List[1].IsFavorited {
		t.Fatalf("expected post 200 not-liked but favorited, got %+v", resp.List[1])
	}
	if resp.List[0].AuthorId != 1 || resp.List[0].AuthorName != "Alice" || resp.List[0].AuthorAvatar != "https://media/alice.png" {
		t.Fatalf("expected post 100 author enriched, got %+v", resp.List[0])
	}
	if resp.List[1].AuthorName != "bob" || resp.List[1].AuthorAvatar != "https://media/bob.png" {
		t.Fatalf("expected empty nickname to fall back to username, got %+v", resp.List[1])
	}
}

func TestGetUserFavorites_NonOwnerUsesViewerFavoriteState(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		UserService: &fakeUserService{
			getUserFn: func(_ context.Context, in *pb.GetUserReq) (*pb.GetUserResp, error) {
				return &pb.GetUserResp{User: &pb.UserInfo{Id: in.UserId, FavoritesVisibility: 1}}, nil
			},
		},
		InteractionService: &fakeInteractionServiceFavorites{
			getFavoriteListFn: func(_ context.Context, in *interactionpb.GetFavoriteListReq, _ ...grpc.CallOption) (*interactionpb.GetFavoriteListResp, error) {
				if in.UserId != 42 {
					t.Fatalf("expected owner 42, got %d", in.UserId)
				}
				return &interactionpb.GetFavoriteListResp{PostIds: []int64{100, 200}, Total: 2}, nil
			},
			liked:     map[int64]bool{200: true},
			favorited: map[int64]bool{100: true},
		},
		ContentService: &fakeContentServiceFavorites{
			getPostsByIdsFn: func(_ context.Context, in *contentpb.GetPostsByIdsReq, _ ...grpc.CallOption) (*contentpb.GetPostsByIdsResp, error) {
				return &contentpb.GetPostsByIdsResp{Posts: []*contentpb.PostInfo{
					{Id: 100, AuthorId: 1, Title: "Post A", Status: 1},
					{Id: 200, AuthorId: 2, Title: "Post B", Status: 1},
				}}, nil
			},
		},
	}

	ctx := jwtx.WithUserIdContext(context.Background(), 7)
	resp, err := NewGetUserFavoritesLogic(ctx, svcCtx).GetUserFavorites(&types.GetUserFavoritesReq{UserId: 42, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.List) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.List))
	}
	if !resp.List[0].IsFavorited || resp.List[0].IsLiked {
		t.Fatalf("viewer should have favorited only post 100, got %+v", resp.List[0])
	}
	if resp.List[1].IsFavorited || !resp.List[1].IsLiked {
		t.Fatalf("viewer should like post 200 without favoriting it, got %+v", resp.List[1])
	}
}

func TestGetUserFavorites_DropsUnavailablePosts(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		UserService: &fakeUserService{
			getUserFn: func(_ context.Context, in *pb.GetUserReq) (*pb.GetUserResp, error) {
				return &pb.GetUserResp{User: &pb.UserInfo{Id: in.UserId, FavoritesVisibility: 1}}, nil
			},
		},
		InteractionService: &fakeInteractionServiceFavorites{
			getFavoriteListFn: func(_ context.Context, _ *interactionpb.GetFavoriteListReq, _ ...grpc.CallOption) (*interactionpb.GetFavoriteListResp, error) {
				return &interactionpb.GetFavoriteListResp{PostIds: []int64{100, 200}, Total: 5}, nil
			},
			favorited: map[int64]bool{100: true},
		},
		ContentService: &fakeContentServiceFavorites{
			getPostsByIdsFn: func(_ context.Context, _ *contentpb.GetPostsByIdsReq, _ ...grpc.CallOption) (*contentpb.GetPostsByIdsResp, error) {
				return &contentpb.GetPostsByIdsResp{Posts: []*contentpb.PostInfo{
					{Id: 100, AuthorId: 1, Title: "live", Status: 1},
				}}, nil
			},
		},
	}

	ctx := jwtx.WithUserIdContext(context.Background(), 42)
	resp, err := NewGetUserFavoritesLogic(ctx, svcCtx).GetUserFavorites(&types.GetUserFavoritesReq{UserId: 42, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.List) != 1 || resp.List[0].Id != 100 {
		t.Fatalf("expected only published favorite, got %+v", resp.List)
	}
	if !resp.List[0].IsFavorited {
		t.Fatalf("owner should still see live favorite as favorited, got %+v", resp.List[0])
	}
}

func TestGetUserFavorites_InteractionRPCError_ReturnsSystemError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		UserService: &fakeUserService{
			getUserFn: func(_ context.Context, in *pb.GetUserReq) (*pb.GetUserResp, error) {
				return &pb.GetUserResp{User: &pb.UserInfo{Id: in.UserId, FavoritesVisibility: 1}}, nil
			},
		},
		InteractionService: &fakeInteractionServiceFavorites{
			getFavoriteListFn: func(_ context.Context, _ *interactionpb.GetFavoriteListReq, _ ...grpc.CallOption) (*interactionpb.GetFavoriteListResp, error) {
				return nil, context.DeadlineExceeded
			},
		},
	}

	ctx := jwtx.WithUserIdContext(context.Background(), 42)
	l := NewGetUserFavoritesLogic(ctx, svcCtx)

	_, err := l.GetUserFavorites(&types.GetUserFavoritesReq{UserId: 42, Page: 1, PageSize: 20})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetUserFavorites_ContentRPCError_ReturnsSystemError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		UserService: &fakeUserService{
			getUserFn: func(_ context.Context, in *pb.GetUserReq) (*pb.GetUserResp, error) {
				return &pb.GetUserResp{User: &pb.UserInfo{Id: in.UserId, FavoritesVisibility: 1}}, nil
			},
		},
		InteractionService: &fakeInteractionServiceFavorites{
			getFavoriteListFn: func(_ context.Context, _ *interactionpb.GetFavoriteListReq, _ ...grpc.CallOption) (*interactionpb.GetFavoriteListResp, error) {
				return &interactionpb.GetFavoriteListResp{PostIds: []int64{100}, Total: 1}, nil
			},
		},
		ContentService: &fakeContentServiceFavorites{
			getPostsByIdsFn: func(_ context.Context, _ *contentpb.GetPostsByIdsReq, _ ...grpc.CallOption) (*contentpb.GetPostsByIdsResp, error) {
				return nil, context.DeadlineExceeded
			},
		},
	}

	ctx := jwtx.WithUserIdContext(context.Background(), 42)
	l := NewGetUserFavoritesLogic(ctx, svcCtx)

	_, err := l.GetUserFavorites(&types.GetUserFavoritesReq{UserId: 42, Page: 1, PageSize: 20})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetUserFavorites_BatchGetUsersError_DegradesToEmptyAuthorFields(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		UserService: &fakeUserService{
			getUserFn: func(_ context.Context, in *pb.GetUserReq) (*pb.GetUserResp, error) {
				return &pb.GetUserResp{User: &pb.UserInfo{Id: in.UserId, FavoritesVisibility: 1}}, nil
			},
			batchGetUsersFn: func(_ context.Context, _ *userservice.BatchGetUsersReq, _ ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
				return nil, context.DeadlineExceeded
			},
		},
		InteractionService: &fakeInteractionServiceFavorites{
			getFavoriteListFn: func(_ context.Context, _ *interactionpb.GetFavoriteListReq, _ ...grpc.CallOption) (*interactionpb.GetFavoriteListResp, error) {
				return &interactionpb.GetFavoriteListResp{PostIds: []int64{100}, Total: 1}, nil
			},
		},
		ContentService: &fakeContentServiceFavorites{
			getPostsByIdsFn: func(_ context.Context, _ *contentpb.GetPostsByIdsReq, _ ...grpc.CallOption) (*contentpb.GetPostsByIdsResp, error) {
				return &contentpb.GetPostsByIdsResp{Posts: []*contentpb.PostInfo{
					{Id: 100, AuthorId: 5, Title: "live", Status: 1},
				}}, nil
			},
		},
	}

	ctx := jwtx.WithUserIdContext(context.Background(), 42)
	resp, err := NewGetUserFavoritesLogic(ctx, svcCtx).GetUserFavorites(&types.GetUserFavoritesReq{UserId: 42, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("expected list to survive author lookup failure, got %v", err)
	}
	if len(resp.List) != 1 || resp.List[0].AuthorName != "" || resp.List[0].AuthorAvatar != "" {
		t.Fatalf("expected empty author fields on BatchGetUsers failure, got %+v", resp.List)
	}
}
