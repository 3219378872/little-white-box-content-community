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

type fakeInteractionServiceUnlike struct {
	interactionservice.InteractionService
	unlikeFn func(ctx context.Context, in *interactionpb.UnlikeReq, opts ...grpc.CallOption) (*interactionpb.UnlikeResp, error)
}

func (f *fakeInteractionServiceUnlike) Unlike(ctx context.Context, in *interactionpb.UnlikeReq, opts ...grpc.CallOption) (*interactionpb.UnlikeResp, error) {
	return f.unlikeFn(ctx, in, opts...)
}

func TestUnlike_Success(t *testing.T) {
	var capturedReq *interactionpb.UnlikeReq
	svcCtx := &svc.ServiceContext{
		InteractionService: &fakeInteractionServiceUnlike{
			unlikeFn: func(_ context.Context, in *interactionpb.UnlikeReq, _ ...grpc.CallOption) (*interactionpb.UnlikeResp, error) {
				capturedReq = in
				return &interactionpb.UnlikeResp{}, nil
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewUnlikeLogic(ctx, svcCtx)

	_, err := l.Unlike(&types.UnlikeReq{TargetId: 100, TargetType: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("expected Unlike RPC to be called")
	}
	if capturedReq.UserId != 42 || capturedReq.TargetId != 100 || capturedReq.TargetType != 1 {
		t.Fatalf("unexpected request: %+v", capturedReq)
	}
}

func TestUnlike_RPCError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		InteractionService: &fakeInteractionServiceUnlike{
			unlikeFn: func(_ context.Context, _ *interactionpb.UnlikeReq, _ ...grpc.CallOption) (*interactionpb.UnlikeResp, error) {
				return nil, context.DeadlineExceeded
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewUnlikeLogic(ctx, svcCtx)

	_, err := l.Unlike(&types.UnlikeReq{TargetId: 100, TargetType: 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
