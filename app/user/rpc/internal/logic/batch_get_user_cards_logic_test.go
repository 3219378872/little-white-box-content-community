package logic

import (
	"context"
	"errors"
	"testing"

	"esx/app/user/rpc/internal/model"
	pb "esx/kitex_gen/user"
	"esx/pkg/errx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestBatchGetUserCardsDeduplicatesAndMapsDisplayFields(t *testing.T) {
	profiles := new(MockUserProfileModel)
	profiles.On("FindCardsByIDs", mock.Anything, []int64{2, 1}).Return([]*model.UserCard{
		{Id: 2, Username: "bob", Nickname: "Bob", AvatarUrl: "https://a/b.png"},
		{Id: 1, Username: "alice"},
	}, nil).Once()

	response, err := NewBatchGetUserCardsLogic(context.Background(), newUnitSvcCtx(profiles, nil)).BatchGetUserCards(
		&pb.BatchGetUserCardsReq{UserIds: []int64{2, 1, 2}},
	)

	require.NoError(t, err)
	require.Len(t, response.Users, 2)
	assert.Equal(t, &pb.UserCard{Id: 2, Username: "bob", Nickname: "Bob", AvatarUrl: "https://a/b.png"}, response.Users[0])
	assert.Equal(t, int64(1), response.Users[1].Id)
	profiles.AssertExpectations(t)
}

func TestBatchGetUserCardsRejectsInvalidRequests(t *testing.T) {
	profiles := new(MockUserProfileModel)
	logic := NewBatchGetUserCardsLogic(context.Background(), newUnitSvcCtx(profiles, nil))
	for _, request := range []*pb.BatchGetUserCardsReq{
		nil, {}, {UserIds: []int64{-1}}, {UserIds: make([]int64, maxBatchGetUsers+1)},
	} {
		response, err := logic.BatchGetUserCards(request)
		require.Error(t, err)
		assert.Equal(t, errx.ParamError, errx.GetCode(err))
		assert.Nil(t, response)
	}
	profiles.AssertNotCalled(t, "FindCardsByIDs", mock.Anything, mock.Anything)
}

func TestBatchGetUserCardsMapsStoreFailure(t *testing.T) {
	profiles := new(MockUserProfileModel)
	profiles.On("FindCardsByIDs", mock.Anything, []int64{1}).Return(
		([]*model.UserCard)(nil), errors.New("database unavailable"),
	).Once()

	response, err := NewBatchGetUserCardsLogic(context.Background(), newUnitSvcCtx(profiles, nil)).BatchGetUserCards(
		&pb.BatchGetUserCardsReq{UserIds: []int64{1}},
	)

	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
	assert.Nil(t, response)
}
