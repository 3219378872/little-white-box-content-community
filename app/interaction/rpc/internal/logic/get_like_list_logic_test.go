package logic

import (
	"context"
	"testing"

	"esx/app/interaction/rpc/internal/svc"
	"esx/app/interaction/rpc/pb/xiaobaihe/interaction/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetLikeListLogic_GetLikeList_DefaultsPagination(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	likeModel.
		On("FindActiveTargetIds", mock.Anything, int64(1), int64(1), int32(1), int32(20)).
		Return([]int64{100, 90}, int64(2), nil).
		Once()

	logic := NewGetLikeListLogic(context.Background(), &svc.ServiceContext{LikeRecordModel: likeModel})
	resp, err := logic.GetLikeList(&pb.GetLikeListReq{UserId: 1, Page: 0, PageSize: 0})
	require.NoError(t, err)
	assert.Equal(t, []int64{100, 90}, resp.PostIds)
	assert.Equal(t, int64(2), resp.Total)
	likeModel.AssertExpectations(t)
}
