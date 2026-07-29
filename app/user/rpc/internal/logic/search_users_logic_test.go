package logic

import (
	"context"
	"errors"
	"strings"
	"testing"

	"errx"
	"user/internal/model"
	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type searchUsersContextKey struct{}

func TestSearchUsersReturnsPublicProfileMatches(t *testing.T) {
	ctx := context.WithValue(context.Background(), searchUsersContextKey{}, "request-context")
	profiles := new(MockUserProfileModel)
	profiles.On("SearchPublic", ctx, "go", int64(20), int64(20)).Return(
		[]*model.UserProfile{sampleUser(2, "gopher")}, int64(21), nil,
	).Once()

	response, err := NewSearchUsersLogic(ctx, newUnitSvcCtx(profiles, nil)).SearchUsers(
		&pb.SearchUsersReq{Keyword: "  go ", Page: 2, PageSize: 20},
	)

	require.NoError(t, err)
	assert.Equal(t, int64(21), response.Total)
	require.Len(t, response.Users, 1)
	assert.Equal(t, "gopher", response.Users[0].Username)
	profiles.AssertExpectations(t)
}

func TestSearchUsersRejectsInvalidRequests(t *testing.T) {
	profiles := new(MockUserProfileModel)
	logic := NewSearchUsersLogic(context.Background(), newUnitSvcCtx(profiles, nil))
	requests := []*pb.SearchUsersReq{
		nil,
		{},
		{Keyword: "go", Page: 0, PageSize: 20},
		{Keyword: "go", Page: 1, PageSize: 101},
		{Keyword: "go", Page: 101, PageSize: 100},
		{Keyword: strings.Repeat("界", 101), Page: 1, PageSize: 20},
	}
	for _, request := range requests {
		response, err := logic.SearchUsers(request)
		require.Error(t, err)
		assert.Equal(t, errx.ParamError, errx.GetCode(err))
		assert.Nil(t, response)
	}
	profiles.AssertNotCalled(t, "SearchPublic", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestSearchUsersMapsStoreFailure(t *testing.T) {
	profiles := new(MockUserProfileModel)
	profiles.On("SearchPublic", mock.Anything, "go", int64(0), int64(20)).Return(
		([]*model.UserProfile)(nil), int64(0), errors.New("database unavailable"),
	).Once()

	response, err := NewSearchUsersLogic(context.Background(), newUnitSvcCtx(profiles, nil)).SearchUsers(
		&pb.SearchUsersReq{Keyword: "go", Page: 1, PageSize: 20},
	)

	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
	assert.Nil(t, response)
	profiles.AssertExpectations(t)
}
