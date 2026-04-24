//go:build integration

package mqs

import (
	"context"
	"testing"

	"esx/app/feed/internal/model"
	"esx/app/feed/internal/svc"
	"user/userservice"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFeedFanout_SmallV_WritesInboxAndOutbox(t *testing.T) {
	conn, cleanup := newFeedTestDB(t)
	defer cleanup()
	userSvc := new(mockUserService)
	userSvc.On("GetUser", mock.Anything, &userservice.GetUserReq{UserId: 9}).Return(&userservice.GetUserResp{User: &userservice.UserInfo{Id: 9, FollowerCount: 99}}, nil).Once()
	userSvc.On("GetFollowers", mock.Anything, &userservice.GetFollowersReq{UserId: 9, Page: 1, PageSize: 500}).Return(&userservice.GetFollowersResp{Users: []*userservice.UserInfo{{Id: 1}, {Id: 2}, {Id: 3}}}, nil).Once()
	svcCtx := &svc.ServiceContext{
		InboxModel:      model.NewFeedInboxModel(conn),
		OutboxModel:     model.NewFeedOutboxModel(conn),
		UserService:     userSvc,
		BigVThreshold:   10000,
		FanoutBatchSize: 500,
	}

	err := handlePostPublished(context.Background(), svcCtx, postPublishedMessage{PostId: 1001, AuthorId: 9, CreatedAt: 1710000000000})

	require.NoError(t, err)
	var outboxCount int64
	require.NoError(t, conn.QueryRowCtx(context.Background(), &outboxCount, "SELECT COUNT(*) FROM feed_outbox"))
	assert.Equal(t, int64(1), outboxCount)
	var inboxCount int64
	require.NoError(t, conn.QueryRowCtx(context.Background(), &inboxCount, "SELECT COUNT(*) FROM feed_inbox"))
	assert.Equal(t, int64(3), inboxCount)
	userSvc.AssertExpectations(t)
}

func TestFeedFanout_BigV_WritesOutboxOnly(t *testing.T) {
	conn, cleanup := newFeedTestDB(t)
	defer cleanup()
	userSvc := new(mockUserService)
	userSvc.On("GetUser", mock.Anything, &userservice.GetUserReq{UserId: 9}).Return(&userservice.GetUserResp{User: &userservice.UserInfo{Id: 9, FollowerCount: 10000}}, nil).Once()
	svcCtx := &svc.ServiceContext{
		InboxModel:      model.NewFeedInboxModel(conn),
		OutboxModel:     model.NewFeedOutboxModel(conn),
		UserService:     userSvc,
		BigVThreshold:   10000,
		FanoutBatchSize: 500,
	}

	err := handlePostPublished(context.Background(), svcCtx, postPublishedMessage{PostId: 1001, AuthorId: 9, CreatedAt: 1710000000000})

	require.NoError(t, err)
	var outboxCount int64
	require.NoError(t, conn.QueryRowCtx(context.Background(), &outboxCount, "SELECT COUNT(*) FROM feed_outbox"))
	assert.Equal(t, int64(1), outboxCount)
	var inboxCount int64
	require.NoError(t, conn.QueryRowCtx(context.Background(), &inboxCount, "SELECT COUNT(*) FROM feed_inbox"))
	assert.Equal(t, int64(0), inboxCount)
	userSvc.AssertExpectations(t)
}
