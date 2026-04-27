package logic

import (
	"context"
	"errors"
	"testing"

	"errx"
	"esx/app/content/rpc/contentservice"
	"esx/app/feed/rpc/internal/model"
	"esx/app/feed/rpc/internal/svc"
	"esx/app/feed/rpc/xiaobaihe/feed/pb"
	"user/userservice"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

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

func TestGetFollowFeedLogic_MergesInboxAndOutbox(t *testing.T) {
	inbox := new(mockInboxModel)
	outbox := new(mockOutboxModel)
	userSvc := new(mockUserService)
	contentSvc := new(mockContentService)
	inbox.On("FindByUserBefore", mock.Anything, int64(1), int64(2000), int64(9999), int64(3)).Return([]*model.FeedInbox{
		{UserId: 1, AuthorId: 9, PostId: 1001, CreatedAt: 1000},
	}, nil).Once()
	userSvc.On("GetFollowing", mock.Anything, &userservice.GetFollowingReq{UserId: 1, Page: 1, PageSize: 1000}).Return(&userservice.GetFollowingResp{Users: []*userservice.UserInfo{{Id: 8}, {Id: 9}}}, nil).Once()
	outbox.On("FindByAuthorsBefore", mock.Anything, []int64{8, 9}, int64(2000), int64(9999), int64(3)).Return([]*model.FeedOutbox{
		{AuthorId: 8, PostId: 1002, CreatedAt: 1001},
		{AuthorId: 9, PostId: 1001, CreatedAt: 1000},
	}, nil).Once()
	contentSvc.On("GetPostsByIds", mock.Anything, &contentservice.GetPostsByIdsReq{PostIds: []int64{1002, 1001}}).Return(&contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{{Id: 1001}, {Id: 1002}}}, nil).Once()

	logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{InboxModel: inbox, OutboxModel: outbox, UserService: userSvc, ContentService: contentSvc})
	resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{UserId: 1, CursorCreatedAt: 2000, CursorPostId: 9999, PageSize: 2})

	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	assert.Equal(t, int64(1002), resp.Items[0].PostId)
	assert.Equal(t, int64(1001), resp.Items[1].PostId)
	assert.True(t, resp.HasMore)
	assert.Equal(t, int64(1000), resp.NextCursorCreatedAt)
	assert.Equal(t, int64(1001), resp.NextCursorPostId)
	inbox.AssertExpectations(t)
	outbox.AssertExpectations(t)
	userSvc.AssertExpectations(t)
	contentSvc.AssertExpectations(t)
}

func TestGetFollowFeedLogic_InvalidInput(t *testing.T) {
	logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{})
	resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{UserId: 0, PageSize: 2})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, errx.ParamError, errx.GetCode(err))
}

func TestGetFollowFeedLogic_DependencyError(t *testing.T) {
	inbox := new(mockInboxModel)
	inbox.On("FindByUserBefore", mock.Anything, int64(1), int64(2000), int64(9999), int64(3)).Return(nil, errors.New("db down")).Once()

	logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{InboxModel: inbox})
	resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{UserId: 1, CursorCreatedAt: 2000, CursorPostId: 9999, PageSize: 2})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
	inbox.AssertExpectations(t)
}
