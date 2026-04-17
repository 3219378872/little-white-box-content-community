package user

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"errx"
	"gateway/internal/svc"
	"gateway/internal/types"
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
	}
	ctx := context.Background()
	if requesterID != 0 {
		// 对齐 go-zero JWT middleware：userId 以 json.Number 形式注入 ctx
		ctx = context.WithValue(ctx, "userId", json.Number(strconv.FormatInt(requesterID, 10)))
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
