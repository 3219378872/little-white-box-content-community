package like_favorite

import (
	"context"
	"testing"

	"esx/app/interaction/rpc/interactionservice"
	interactionpb "esx/app/interaction/rpc/pb/xiaobaihe/interaction/pb"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"

	"google.golang.org/grpc"
)

type fakeInteractionServiceUnfavorite struct {
	interactionservice.InteractionService
	unfavoriteFn func(ctx context.Context, in *interactionpb.UnfavoriteReq, opts ...grpc.CallOption) (*interactionpb.UnfavoriteResp, error)
}

func (f *fakeInteractionServiceUnfavorite) Unfavorite(ctx context.Context, in *interactionpb.UnfavoriteReq, opts ...grpc.CallOption) (*interactionpb.UnfavoriteResp, error) {
	return f.unfavoriteFn(ctx, in, opts...)
}

func TestUnfavorite_Success(t *testing.T) {
	var capturedReq *interactionpb.UnfavoriteReq
	svcCtx := &svc.ServiceContext{
		InteractionService: &fakeInteractionServiceUnfavorite{
			unfavoriteFn: func(_ context.Context, in *interactionpb.UnfavoriteReq, _ ...grpc.CallOption) (*interactionpb.UnfavoriteResp, error) {
				capturedReq = in
				return &interactionpb.UnfavoriteResp{}, nil
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewUnfavoriteLogic(ctx, svcCtx)

	_, err := l.Unfavorite(&types.UnfavoriteReq{PostId: 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("expected Unfavorite RPC to be called")
	}
	if capturedReq.UserId != 42 || capturedReq.PostId != 100 {
		t.Fatalf("unexpected request: %+v", capturedReq)
	}
}

func TestUnfavorite_RPCError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		InteractionService: &fakeInteractionServiceUnfavorite{
			unfavoriteFn: func(_ context.Context, _ *interactionpb.UnfavoriteReq, _ ...grpc.CallOption) (*interactionpb.UnfavoriteResp, error) {
				return nil, context.DeadlineExceeded
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewUnfavoriteLogic(ctx, svcCtx)

	_, err := l.Unfavorite(&types.UnfavoriteReq{PostId: 100})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
