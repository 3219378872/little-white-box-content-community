package user

import (
	"context"
	"testing"

	"errx"
	"esx/app/content/rpc/contentservice"
	contentpb "esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/app/interaction/rpc/interactionservice"
	interactionpb "esx/app/interaction/rpc/pb/xiaobaihe/interaction/pb"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"
	"user/pb/xiaobaihe/user/pb"
	"user/userservice"

	"google.golang.org/grpc"
)

type fakeUserService struct {
	userservice.UserService
	getUserFn func(ctx context.Context, in *pb.GetUserReq) (*pb.GetUserResp, error)
}

func (f *fakeUserService) GetUser(ctx context.Context, in *pb.GetUserReq, _ ...grpc.CallOption) (*pb.GetUserResp, error) {
	return f.getUserFn(ctx, in)
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
	if resp.Total != 0 {
		t.Fatalf("expected Total=0, got %d", resp.Total)
	}
	if resp.Page != 1 || resp.PageSize != 20 {
		t.Fatalf("expected Page=1, PageSize=20, got Page=%d, PageSize=%d", resp.Page, resp.PageSize)
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
}

func (f *fakeInteractionServiceFavorites) GetFavoriteList(ctx context.Context, in *interactionpb.GetFavoriteListReq, opts ...grpc.CallOption) (*interactionpb.GetFavoriteListResp, error) {
	return f.getFavoriteListFn(ctx, in, opts...)
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
		},
		ContentService: &fakeContentServiceFavorites{
			getPostsByIdsFn: func(_ context.Context, in *contentpb.GetPostsByIdsReq, _ ...grpc.CallOption) (*contentpb.GetPostsByIdsResp, error) {
				if len(in.PostIds) != 2 {
					t.Fatalf("expected 2 post ids, got %d", len(in.PostIds))
				}
				return &contentpb.GetPostsByIdsResp{
					Posts: []*contentpb.PostInfo{
						{Id: 100, AuthorId: 1, Title: "Post A", Content: "Content A", ViewCount: 10, LikeCount: 5, CommentCount: 2, FavoriteCount: 3, CreatedAt: 1000},
						{Id: 200, AuthorId: 2, Title: "Post B", Content: "Content B", ViewCount: 20, LikeCount: 10, CommentCount: 4, FavoriteCount: 6, CreatedAt: 2000},
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
	if resp.Total != 2 {
		t.Fatalf("expected Total=2, got %d", resp.Total)
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
