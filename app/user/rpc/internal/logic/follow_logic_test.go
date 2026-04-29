package logic

import (
	"context"
	"testing"

	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/require"
)

func TestFollowLogic_Stub(t *testing.T) {
	logic := NewFollowLogic(context.Background(), nil)
	resp, err := logic.Follow(&pb.FollowReq{UserId: 1, TargetUserId: 2})

	require.NoError(t, err)
	require.NotNil(t, resp)
}
