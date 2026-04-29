package logic

import (
	"context"
	"testing"

	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/require"
)

func TestUnfollowLogic_Stub(t *testing.T) {
	logic := NewUnfollowLogic(context.Background(), nil)
	resp, err := logic.Unfollow(&pb.UnfollowReq{UserId: 1, TargetUserId: 2})

	require.NoError(t, err)
	require.NotNil(t, resp)
}
