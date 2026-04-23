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

type fakeInteractionService struct {
	interactionservice.InteractionService
	likeFn func(ctx context.Context, in *interactionpb.LikeReq, opts ...grpc.CallOption) (*interactionpb.LikeResp, error)
}

func (f *fakeInteractionService) Like(ctx context.Context, in *interactionpb.LikeReq, opts ...grpc.CallOption) (*interactionpb.LikeResp, error) {
	return f.likeFn(ctx, in, opts...)
}

func TestLike_Success(t *testing.T) {
	var capturedReq *interactionpb.LikeReq
	svcCtx := &svc.ServiceContext{
		InteractionService: &fakeInteractionService{
			likeFn: func(_ context.Context, in *interactionpb.LikeReq, _ ...grpc.CallOption) (*interactionpb.LikeResp, error) {
				capturedReq = in
				return &interactionpb.LikeResp{}, nil
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewLikeLogic(ctx, svcCtx)

	_, err := l.Like(&types.LikeReq{TargetId: 100, TargetType: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("expected Like RPC to be called")
	}
	if capturedReq.UserId != 42 || capturedReq.TargetId != 100 || capturedReq.TargetType != 1 {
		t.Fatalf("unexpected request: %+v", capturedReq)
	}
}

func TestLike_RPCError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		InteractionService: &fakeInteractionService{
			likeFn: func(_ context.Context, _ *interactionpb.LikeReq, _ ...grpc.CallOption) (*interactionpb.LikeResp, error) {
				return nil, context.DeadlineExceeded
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewLikeLogic(ctx, svcCtx)

	_, err := l.Like(&types.LikeReq{TargetId: 100, TargetType: 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
