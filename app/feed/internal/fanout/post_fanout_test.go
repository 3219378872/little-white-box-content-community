package fanout

import (
	"context"
	"testing"

	"esx/app/feed/internal/model"
	"esx/app/feed/internal/svc"
	"user/userservice"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type mockInboxModel struct{ mock.Mock }

func (m *mockInboxModel) BatchInsertIgnore(ctx context.Context, rows []*model.FeedInbox) (int64, error) {
	args := m.Called(ctx, rows)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockInboxModel) FindByUserBefore(ctx context.Context, userID, cursorCreatedAt, cursorPostID int64, limit int64) ([]*model.FeedInbox, error) {
	args := m.Called(ctx, userID, cursorCreatedAt, cursorPostID, limit)
	if v := args.Get(0); v != nil {
		return v.([]*model.FeedInbox), args.Error(1)
	}
	return nil, args.Error(1)
}

type mockOutboxModel struct{ mock.Mock }

func (m *mockOutboxModel) InsertIgnore(ctx context.Context, row *model.FeedOutbox) error {
	return m.Called(ctx, row).Error(0)
}

func (m *mockOutboxModel) FindByAuthorsBefore(ctx context.Context, authorIDs []int64, cursorCreatedAt, cursorPostID int64, limit int64) ([]*model.FeedOutbox, error) {
	args := m.Called(ctx, authorIDs, cursorCreatedAt, cursorPostID, limit)
	if v := args.Get(0); v != nil {
		return v.([]*model.FeedOutbox), args.Error(1)
	}
	return nil, args.Error(1)
}

type mockUserService struct{ mock.Mock }

func (m *mockUserService) GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...grpc.CallOption) (*userservice.GetUserResp, error) {
	args := m.Called(ctx, in)
	if v := args.Get(0); v != nil {
		return v.(*userservice.GetUserResp), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUserService) GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...grpc.CallOption) (*userservice.GetFollowersResp, error) {
	args := m.Called(ctx, in)
	if v := args.Get(0); v != nil {
		return v.(*userservice.GetFollowersResp), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUserService) GetFollowing(ctx context.Context, in *userservice.GetFollowingReq, opts ...grpc.CallOption) (*userservice.GetFollowingResp, error) {
	args := m.Called(ctx, in)
	if v := args.Get(0); v != nil {
		return v.(*userservice.GetFollowingResp), args.Error(1)
	}
	return nil, args.Error(1)
}

func TestHandlePostPublished_SmallVFanout(t *testing.T) {
	inbox := new(mockInboxModel)
	outbox := new(mockOutboxModel)
	userSvc := new(mockUserService)
	event := PostPublished{PostId: 1001, AuthorId: 9, CreatedAt: 1710000000000}
	userSvc.On("GetUser", mock.Anything, &userservice.GetUserReq{UserId: 9}).Return(&userservice.GetUserResp{User: &userservice.UserInfo{Id: 9, FollowerCount: 99}}, nil).Once()
	outbox.On("InsertIgnore", mock.Anything, &model.FeedOutbox{AuthorId: 9, PostId: 1001, CreatedAt: 1710000000000}).Return(nil).Once()
	userSvc.On("GetFollowers", mock.Anything, &userservice.GetFollowersReq{UserId: 9, Page: 1, PageSize: 500}).Return(&userservice.GetFollowersResp{Users: []*userservice.UserInfo{{Id: 1}, {Id: 2}, {Id: 3}}}, nil).Once()
	inbox.On("BatchInsertIgnore", mock.Anything, mock.MatchedBy(func(rows []*model.FeedInbox) bool {
		return len(rows) == 3 && rows[0].UserId == 1 && rows[1].UserId == 2 && rows[2].UserId == 3
	})).Return(int64(3), nil).Once()

	pushed, err := HandlePostPublished(context.Background(), &svc.ServiceContext{InboxModel: inbox, OutboxModel: outbox, UserService: userSvc, BigVThreshold: 10000, FanoutBatchSize: 500}, event)

	require.NoError(t, err)
	require.Equal(t, int64(3), pushed)
	inbox.AssertExpectations(t)
	outbox.AssertExpectations(t)
	userSvc.AssertExpectations(t)
}

func TestHandlePostPublished_BigVOutboxOnly(t *testing.T) {
	inbox := new(mockInboxModel)
	outbox := new(mockOutboxModel)
	userSvc := new(mockUserService)
	event := PostPublished{PostId: 1001, AuthorId: 9, CreatedAt: 1710000000000}
	userSvc.On("GetUser", mock.Anything, &userservice.GetUserReq{UserId: 9}).Return(&userservice.GetUserResp{User: &userservice.UserInfo{Id: 9, FollowerCount: 10000}}, nil).Once()
	outbox.On("InsertIgnore", mock.Anything, &model.FeedOutbox{AuthorId: 9, PostId: 1001, CreatedAt: 1710000000000}).Return(nil).Once()

	pushed, err := HandlePostPublished(context.Background(), &svc.ServiceContext{InboxModel: inbox, OutboxModel: outbox, UserService: userSvc, BigVThreshold: 10000, FanoutBatchSize: 500}, event)

	require.NoError(t, err)
	require.Zero(t, pushed)
	inbox.AssertNotCalled(t, "BatchInsertIgnore", mock.Anything, mock.Anything)
	outbox.AssertExpectations(t)
	userSvc.AssertExpectations(t)
}
