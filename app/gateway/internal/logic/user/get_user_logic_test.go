package user

import (
	"context"
	"testing"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/stretchr/testify/require"
)

func TestGetUserPassesAuthenticatedViewer(t *testing.T) {
	for _, viewer := range []int64{0, 7} {
		ctx := jwtx.WithUserIdContext(context.Background(), viewer)
		service := &fakeUserService{getUserFn: func(ctx context.Context, req *pb.GetUserReq) (*pb.GetUserResp, error) {
			require.Equal(t, int64(2), req.UserId)
			require.Equal(t, viewer, req.ViewerId)
			return &pb.GetUserResp{User: &pb.UserInfo{Id: 2}, IsFollowing: viewer == 7}, nil
		}}
		response, err := NewGetUserLogic(ctx, &svc.ServiceContext{UserService: service}).GetUser(&types.GetUserReq{UserId: 2})
		require.NoError(t, err)
		require.Equal(t, viewer == 7, response.IsFollowing)
	}
}

func TestGetUserRelationFailureDoesNotReturnFalseSuccess(t *testing.T) {
	service := &fakeUserService{getUserFn: func(context.Context, *pb.GetUserReq) (*pb.GetUserResp, error) {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}}
	response, err := NewGetUserLogic(jwtx.WithUserIdContext(context.Background(), 7), &svc.ServiceContext{UserService: service}).GetUser(&types.GetUserReq{UserId: 2})
	require.Error(t, err)
	require.Nil(t, response)
}
