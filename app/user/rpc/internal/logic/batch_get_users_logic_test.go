package logic

import (
	"context"
	"errors"
	"testing"

	"esx/app/user/rpc/internal/model"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/errx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type batchGetUsersContextKey struct{}

func TestBatchGetUsersDeduplicatesAndPreservesRequestOrder(t *testing.T) {
	ctx := context.WithValue(context.Background(), batchGetUsersContextKey{}, "request-context")
	profiles := new(MockUserProfileModel)
	profiles.On("FindByIDs", ctx, []int64{2, 1}).Return([]*model.UserProfile{
		sampleUser(1, "alice"),
		sampleUser(2, "bob"),
	}, nil).Once()

	response, err := NewBatchGetUsersLogic(ctx, newUnitSvcCtx(profiles, nil)).BatchGetUsers(
		&pb.BatchGetUsersReq{UserIds: []int64{2, 1, 2}},
	)
	require.NoError(t, err)
	require.Len(t, response.Users, 2)
	assert.Equal(t, []int64{2, 1}, []int64{response.Users[0].Id, response.Users[1].Id})
	profiles.AssertExpectations(t)
}

func TestBatchGetUsersRejectsInvalidRequests(t *testing.T) {
	profiles := new(MockUserProfileModel)
	logic := NewBatchGetUsersLogic(context.Background(), newUnitSvcCtx(profiles, nil))
	requests := []*pb.BatchGetUsersReq{
		nil,
		{},
		{UserIds: []int64{0}},
		{UserIds: make([]int64, maxBatchGetUsers+1)},
	}
	for _, request := range requests {
		response, err := logic.BatchGetUsers(request)
		require.Error(t, err)
		assert.Equal(t, errx.ParamError, errx.GetCode(err))
		assert.Nil(t, response)
	}
	profiles.AssertNotCalled(t, "FindByIDs", mock.Anything, mock.Anything)
}

func TestBatchGetUsersMapsStoreFailure(t *testing.T) {
	profiles := new(MockUserProfileModel)
	profiles.On("FindByIDs", mock.Anything, []int64{1}).Return(
		([]*model.UserProfile)(nil), errors.New("database unavailable"),
	).Once()

	response, err := NewBatchGetUsersLogic(context.Background(), newUnitSvcCtx(profiles, nil)).BatchGetUsers(
		&pb.BatchGetUsersReq{UserIds: []int64{1}},
	)
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
	assert.Nil(t, response)
	profiles.AssertExpectations(t)
}
