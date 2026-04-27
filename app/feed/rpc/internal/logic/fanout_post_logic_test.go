package logic

import (
	"context"
	"testing"

	"errx"
	"esx/app/feed/rpc/internal/model"
	"esx/app/feed/rpc/internal/svc"
	"esx/app/feed/rpc/xiaobaihe/feed/pb"
	"user/userservice"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFanoutPostLogic_SmallVFanout(t *testing.T) {
	inbox := new(mockInboxModel)
	outbox := new(mockOutboxModel)
	userSvc := new(mockUserService)
	userSvc.On("GetUser", mock.Anything, &userservice.GetUserReq{UserId: 9}).Return(&userservice.GetUserResp{User: &userservice.UserInfo{Id: 9, FollowerCount: 99}}, nil).Once()
	outbox.On("InsertIgnore", mock.Anything, &model.FeedOutbox{AuthorId: 9, PostId: 1001, CreatedAt: 1710000000000}).Return(nil).Once()
	userSvc.On("GetFollowers", mock.Anything, &userservice.GetFollowersReq{UserId: 9, Page: 1, PageSize: 500}).Return(&userservice.GetFollowersResp{Users: []*userservice.UserInfo{{Id: 1}, {Id: 2}}}, nil).Once()
	inbox.On("BatchInsertIgnore", mock.Anything, mock.MatchedBy(func(rows []*model.FeedInbox) bool {
		return len(rows) == 2 && rows[0].UserId == 1 && rows[1].UserId == 2
	})).Return(int64(2), nil).Once()
	logic := NewFanoutPostLogic(context.Background(), &svc.ServiceContext{InboxModel: inbox, OutboxModel: outbox, UserService: userSvc, BigVThreshold: 10000, FanoutBatchSize: 500})

	resp, err := logic.FanoutPost(&pb.FanoutPostReq{AuthorId: 9, PostId: 1001, CreatedAt: 1710000000000})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(2), resp.PushedCount)
	inbox.AssertExpectations(t)
	outbox.AssertExpectations(t)
	userSvc.AssertExpectations(t)
}

func TestFanoutPostLogic_InvalidInput(t *testing.T) {
	logic := NewFanoutPostLogic(context.Background(), &svc.ServiceContext{})

	resp, err := logic.FanoutPost(&pb.FanoutPostReq{AuthorId: 0, PostId: 1001, CreatedAt: 1710000000000})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, errx.ParamError, errx.GetCode(err))
}
