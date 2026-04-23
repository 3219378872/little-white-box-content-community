package like_favorite

import (
	"context"
	"testing"

	"esx/app/interaction/interactionservice"
	interactionpb "esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"

	"google.golang.org/grpc"
)

type fakeInteractionServiceFavorite struct {
	interactionservice.InteractionService
	favoriteFn func(ctx context.Context, in *interactionpb.FavoriteReq, opts ...grpc.CallOption) (*interactionpb.FavoriteResp, error)
}

func (f *fakeInteractionServiceFavorite) Favorite(ctx context.Context, in *interactionpb.FavoriteReq, opts ...grpc.CallOption) (*interactionpb.FavoriteResp, error) {
	return f.favoriteFn(ctx, in, opts...)
}

func TestFavorite_Success(t *testing.T) {
	var capturedReq *interactionpb.FavoriteReq
	svcCtx := &svc.ServiceContext{
		InteractionService: &fakeInteractionServiceFavorite{
			favoriteFn: func(_ context.Context, in *interactionpb.FavoriteReq, _ ...grpc.CallOption) (*interactionpb.FavoriteResp, error) {
				capturedReq = in
				return &interactionpb.FavoriteResp{}, nil
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewFavoriteLogic(ctx, svcCtx)

	_, err := l.Favorite(&types.FavoriteReq{PostId: 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("expected Favorite RPC to be called")
	}
	if capturedReq.UserId != 42 || capturedReq.PostId != 100 {
		t.Fatalf("unexpected request: %+v", capturedReq)
	}
}

func TestFavorite_RPCError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		InteractionService: &fakeInteractionServiceFavorite{
			favoriteFn: func(_ context.Context, _ *interactionpb.FavoriteReq, _ ...grpc.CallOption) (*interactionpb.FavoriteResp, error) {
				return nil, context.DeadlineExceeded
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewFavoriteLogic(ctx, svcCtx)

	_, err := l.Favorite(&types.FavoriteReq{PostId: 100})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
