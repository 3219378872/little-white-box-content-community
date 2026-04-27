package logic

import (
	"context"
	"esx/app/interaction/rpc/internal/svc"
	"esx/app/interaction/rpc/pb/xiaobaihe/interaction/pb"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetFavoriteListLogic_GetFavoriteList_DefaultsPagination(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	favoriteModel.
		On("FindActivePostIds", mock.Anything, int64(1), int32(1), int32(20)).
		Return([]int64{100, 90}, int64(2), nil).
		Once()

	logic := NewGetFavoriteListLogic(context.Background(), &svc.ServiceContext{FavoriteModel: favoriteModel})
	resp, err := logic.GetFavoriteList(&pb.GetFavoriteListReq{UserId: 1, Page: 0, PageSize: 0})
	require.NoError(t, err)
	assert.Equal(t, []int64{100, 90}, resp.PostIds)
	assert.Equal(t, int64(2), resp.Total)
	favoriteModel.AssertExpectations(t)
}
