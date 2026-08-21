package logic

import (
	"context"
	"errors"
	"math"
	"testing"

	"esx/app/content/rpc/contentservice"
	"esx/app/feed/rpc/internal/model"
	"esx/app/feed/rpc/internal/svc"
	"esx/app/feed/rpc/xiaobaihe/feed/pb"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"

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

func followingUsers(ids ...int64) []*userservice.UserInfo {
	users := make([]*userservice.UserInfo, 0, len(ids))
	for _, id := range ids {
		users = append(users, &userservice.UserInfo{Id: id})
	}
	return users
}

func TestGetFollowFeedLogic_MergesInboxAndOutbox(t *testing.T) {
	inbox := new(mockInboxModel)
	outbox := new(mockOutboxModel)
	userSvc := new(mockUserService)
	contentSvc := new(mockContentService)
	userSvc.On("GetFollowing", mock.Anything, &userservice.GetFollowingReq{UserId: 1, Page: 1, PageSize: followingLookupPageSize}).
		Return(&userservice.GetFollowingResp{Users: followingUsers(8, 9), Total: 2}, nil).Once()
	inbox.On("FindByUserBefore", mock.Anything, int64(1), int64(2000), int64(9999), int64(3)).Return([]*model.FeedInbox{
		{UserId: 1, AuthorId: 9, PostId: 1001, CreatedAt: 1000},
	}, nil).Once()
	outbox.On("FindByAuthorsBefore", mock.Anything, []int64{8, 9}, int64(2000), int64(9999), int64(3)).Return([]*model.FeedOutbox{
		{AuthorId: 8, PostId: 1002, CreatedAt: 1001},
		{AuthorId: 9, PostId: 1001, CreatedAt: 1000},
	}, nil).Once()
	contentSvc.On("GetPostsByIds", mock.Anything, &contentservice.GetPostsByIdsReq{PostIds: []int64{1002, 1001}}).Return(&contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{
		{Id: 1001, AuthorId: 9, Title: "second", Content: "second content", Status: 1, CreatedAt: 1000},
		{
			Id: 1002, AuthorId: 8, Title: "first", Content: "first content", Status: 1,
			Images: []string{"first.png"}, Tags: []string{"go", "feed"}, ViewCount: 10,
			LikeCount: 9, CommentCount: 8, FavoriteCount: 7, CreatedAt: 1001,
		},
	}}, nil).Once()

	logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{InboxModel: inbox, OutboxModel: outbox, UserService: userSvc, ContentService: contentSvc})
	resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{UserId: 1, CursorCreatedAt: 2000, CursorPostId: 9999, PageSize: 2})

	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	first := resp.Items[0]
	assert.Equal(t, int64(1002), first.PostId)
	assert.Equal(t, int64(8), first.AuthorId)
	assert.Equal(t, int64(1001), first.CreatedAt)
	assert.Equal(t, int32(feedTypeFollow), first.FeedType)
	assert.Equal(t, "first", first.Title)
	assert.Equal(t, "first content", first.Content)
	assert.Equal(t, []string{"first.png"}, first.Images)
	assert.Equal(t, []string{"go", "feed"}, first.Tags)
	assert.Equal(t, int64(10), first.ViewCount)
	assert.Equal(t, int64(9), first.LikeCount)
	assert.Equal(t, int64(8), first.CommentCount)
	assert.Equal(t, int64(7), first.FavoriteCount)
	assert.Equal(t, int64(1001), resp.Items[1].PostId)
	assert.NotNil(t, resp.Items[1].Images)
	assert.NotNil(t, resp.Items[1].Tags)
	assert.False(t, resp.HasMore)
	assert.Equal(t, int64(1000), resp.NextCursorCreatedAt)
	assert.Equal(t, int64(1001), resp.NextCursorPostId)
	inbox.AssertExpectations(t)
	outbox.AssertExpectations(t)
	userSvc.AssertExpectations(t)
	contentSvc.AssertExpectations(t)
}

func TestGetFollowFeedLogic_InvalidInput(t *testing.T) {
	tests := []*pb.GetFollowFeedReq{
		nil,
		{UserId: 0, PageSize: 2},
		{UserId: 1, PageSize: 0},
		{UserId: 1, PageSize: maxFeedPageSize + 1},
	}
	for _, request := range tests {
		logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{})
		resp, err := logic.GetFollowFeed(request)

		require.Nil(t, resp)
		require.Error(t, err)
		assert.Equal(t, errx.ParamError, errx.GetCode(err))
	}
}

// DISC-012：无关注内容时返回合法空结果，不混入推荐或热门内容。
func TestGetFollowFeedLogic_EmptyFollowingReturnsEmpty(t *testing.T) {
	inbox := new(mockInboxModel)
	outbox := new(mockOutboxModel)
	userSvc := new(mockUserService)
	contentSvc := new(mockContentService)
	userSvc.On("GetFollowing", mock.Anything, &userservice.GetFollowingReq{UserId: 7, Page: 1, PageSize: followingLookupPageSize}).
		Return(&userservice.GetFollowingResp{Users: []*userservice.UserInfo{}, Total: 0}, nil).Once()

	logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{
		InboxModel: inbox, OutboxModel: outbox, UserService: userSvc, ContentService: contentSvc,
	})
	resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{UserId: 7, PageSize: 20})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Items, "关注流为空时必须返回空结果，不混入推荐内容")
	assert.False(t, resp.HasMore)
	inbox.AssertNotCalled(t, "FindByUserBefore", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	outbox.AssertNotCalled(t, "FindByAuthorsBefore", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	contentSvc.AssertNotCalled(t, "GetPostsByIds", mock.Anything, mock.Anything)
}

func TestGetFollowFeedLogic_ExcludesUnfollowedInboxAuthors(t *testing.T) {
	inbox := new(mockInboxModel)
	outbox := new(mockOutboxModel)
	userSvc := new(mockUserService)
	contentSvc := new(mockContentService)
	userSvc.On("GetFollowing", mock.Anything, &userservice.GetFollowingReq{UserId: 1, Page: 1, PageSize: followingLookupPageSize}).
		Return(&userservice.GetFollowingResp{Users: followingUsers(8), Total: 1}, nil).Once()
	inbox.On("FindByUserBefore", mock.Anything, int64(1), int64(4000), int64(9999), int64(3)).Return([]*model.FeedInbox{
		{UserId: 1, AuthorId: 9, PostId: 300, CreatedAt: 3000},
		{UserId: 1, AuthorId: 9, PostId: 200, CreatedAt: 2000},
	}, nil).Once()
	outbox.On("FindByAuthorsBefore", mock.Anything, []int64{8}, int64(4000), int64(9999), int64(3)).
		Return([]*model.FeedOutbox{}, nil).Once()

	logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{
		InboxModel: inbox, OutboxModel: outbox, UserService: userSvc, ContentService: contentSvc,
	})
	resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{
		UserId: 1, CursorCreatedAt: 4000, CursorPostId: 9999, PageSize: 2,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Items)
	assert.False(t, resp.HasMore)
	contentSvc.AssertNotCalled(t, "GetPostsByIds", mock.Anything, mock.Anything)
	inbox.AssertExpectations(t)
	outbox.AssertExpectations(t)
	userSvc.AssertExpectations(t)
}

func TestGetFollowFeedLogic_PaginatesFollowingAndBatchesOutbox(t *testing.T) {
	inbox := new(mockInboxModel)
	outbox := new(mockOutboxModel)
	userSvc := new(mockUserService)
	contentSvc := new(mockContentService)

	page1 := make([]int64, 0, followingLookupPageSize)
	for id := int64(1); id <= followingLookupPageSize; id++ {
		page1 = append(page1, id)
	}
	page2 := []int64{int64(followingLookupPageSize + 1)}
	userSvc.On("GetFollowing", mock.Anything, &userservice.GetFollowingReq{UserId: 1, Page: 1, PageSize: followingLookupPageSize}).
		Return(&userservice.GetFollowingResp{Users: followingUsers(page1...), Total: int64(followingLookupPageSize + 1)}, nil).Once()
	userSvc.On("GetFollowing", mock.Anything, &userservice.GetFollowingReq{UserId: 1, Page: 2, PageSize: followingLookupPageSize}).
		Return(&userservice.GetFollowingResp{Users: followingUsers(page2...), Total: int64(followingLookupPageSize + 1)}, nil).Once()
	inbox.On("FindByUserBefore", mock.Anything, int64(1), int64(2000), int64(9999), int64(3)).
		Return([]*model.FeedInbox{}, nil).Once()
	outbox.On("FindByAuthorsBefore", mock.Anything, page1, int64(2000), int64(9999), int64(3)).
		Return([]*model.FeedOutbox{{AuthorId: 1, PostId: 11, CreatedAt: 1100}}, nil).Once()
	outbox.On("FindByAuthorsBefore", mock.Anything, page2, int64(2000), int64(9999), int64(3)).
		Return([]*model.FeedOutbox{{AuthorId: followingLookupPageSize + 1, PostId: 22, CreatedAt: 1200}}, nil).Once()
	contentSvc.On("GetPostsByIds", mock.Anything, &contentservice.GetPostsByIdsReq{PostIds: []int64{22, 11}}).
		Return(&contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{
			{Id: 22, AuthorId: followingLookupPageSize + 1, Title: "newer", Status: 1, CreatedAt: 1200},
			{Id: 11, AuthorId: 1, Title: "older", Status: 1, CreatedAt: 1100},
		}}, nil).Once()

	logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{
		InboxModel: inbox, OutboxModel: outbox, UserService: userSvc, ContentService: contentSvc,
	})
	resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{UserId: 1, CursorCreatedAt: 2000, CursorPostId: 9999, PageSize: 2})

	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	assert.Equal(t, int64(22), resp.Items[0].PostId)
	assert.Equal(t, int64(11), resp.Items[1].PostId)
	userSvc.AssertExpectations(t)
	outbox.AssertExpectations(t)
	contentSvc.AssertExpectations(t)
}

func TestGetFollowFeedLogic_FiltersUnavailablePostsAndAdvancesScannedCursor(t *testing.T) {
	inbox := new(mockInboxModel)
	outbox := new(mockOutboxModel)
	userSvc := new(mockUserService)
	contentSvc := new(mockContentService)
	userSvc.On("GetFollowing", mock.Anything, &userservice.GetFollowingReq{UserId: 1, Page: 1, PageSize: followingLookupPageSize}).
		Return(&userservice.GetFollowingResp{Users: followingUsers(9), Total: 1}, nil).Once()
	inbox.On("FindByUserBefore", mock.Anything, int64(1), int64(4000), int64(9999), int64(3)).Return([]*model.FeedInbox{
		{UserId: 1, AuthorId: 9, PostId: 300, CreatedAt: 3000},
		{UserId: 1, AuthorId: 9, PostId: 200, CreatedAt: 2000},
		{UserId: 1, AuthorId: 9, PostId: 100, CreatedAt: 1000},
	}, nil).Once()
	outbox.On("FindByAuthorsBefore", mock.Anything, []int64{9}, int64(4000), int64(9999), int64(3)).
		Return([]*model.FeedOutbox{}, nil).Once()
	contentSvc.On("GetPostsByIds", mock.Anything, &contentservice.GetPostsByIdsReq{PostIds: []int64{300, 200, 100}}).Return(&contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{
		{Id: 300, AuthorId: 9, Title: "published", Status: 1, CreatedAt: 3000},
		{Id: 200, AuthorId: 9, Title: "unpublished", Status: 2, CreatedAt: 2000},
		{Id: 999, AuthorId: 9, Title: "not requested", Status: 1, CreatedAt: 999},
	}}, nil).Once()

	logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{
		InboxModel: inbox, OutboxModel: outbox, UserService: userSvc, ContentService: contentSvc,
	})
	resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{
		UserId: 1, CursorCreatedAt: 4000, CursorPostId: 9999, PageSize: 2,
	})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, int64(300), resp.Items[0].PostId)
	assert.True(t, resp.HasMore)
	assert.Equal(t, int64(1000), resp.NextCursorCreatedAt)
	assert.Equal(t, int64(100), resp.NextCursorPostId)
	inbox.AssertExpectations(t)
	outbox.AssertExpectations(t)
	userSvc.AssertExpectations(t)
	contentSvc.AssertExpectations(t)
}

func TestGetFollowFeedLogic_ContentFailure(t *testing.T) {
	inbox := new(mockInboxModel)
	outbox := new(mockOutboxModel)
	userSvc := new(mockUserService)
	contentSvc := new(mockContentService)
	userSvc.On("GetFollowing", mock.Anything, &userservice.GetFollowingReq{UserId: 1, Page: 1, PageSize: followingLookupPageSize}).
		Return(&userservice.GetFollowingResp{Users: followingUsers(9), Total: 1}, nil).Once()
	inbox.On("FindByUserBefore", mock.Anything, int64(1), int64(2000), int64(9999), int64(3)).Return([]*model.FeedInbox{
		{UserId: 1, AuthorId: 9, PostId: 1001, CreatedAt: 1000},
	}, nil).Once()
	outbox.On("FindByAuthorsBefore", mock.Anything, []int64{9}, int64(2000), int64(9999), int64(3)).
		Return([]*model.FeedOutbox{}, nil).Once()
	contentSvc.On("GetPostsByIds", mock.Anything, &contentservice.GetPostsByIdsReq{PostIds: []int64{1001}}).Return(nil, errors.New("content unavailable")).Once()

	logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{
		InboxModel: inbox, OutboxModel: outbox, UserService: userSvc, ContentService: contentSvc,
	})
	resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{
		UserId: 1, CursorCreatedAt: 2000, CursorPostId: 9999, PageSize: 2,
	})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
	contentSvc.AssertExpectations(t)
}

func TestGetFollowFeedLogic_FollowingError(t *testing.T) {
	userSvc := new(mockUserService)
	userSvc.On("GetFollowing", mock.Anything, &userservice.GetFollowingReq{UserId: 1, Page: 1, PageSize: followingLookupPageSize}).
		Return(nil, errors.New("user down")).Once()

	logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{UserService: userSvc})
	resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{UserId: 1, CursorCreatedAt: 2000, CursorPostId: 9999, PageSize: 2})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
	userSvc.AssertExpectations(t)
}

func TestGetFollowFeedLogic_DependencyError(t *testing.T) {
	inbox := new(mockInboxModel)
	userSvc := new(mockUserService)
	userSvc.On("GetFollowing", mock.Anything, &userservice.GetFollowingReq{UserId: 1, Page: 1, PageSize: followingLookupPageSize}).
		Return(&userservice.GetFollowingResp{Users: followingUsers(9), Total: 1}, nil).Once()
	inbox.On("FindByUserBefore", mock.Anything, int64(1), int64(2000), int64(9999), int64(3)).Return(nil, errors.New("db down")).Once()

	logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{InboxModel: inbox, UserService: userSvc})
	resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{UserId: 1, CursorCreatedAt: 2000, CursorPostId: 9999, PageSize: 2})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
	inbox.AssertExpectations(t)
}

func TestGetFollowFeedLogic_FullUnfollowedInboxPageStillAdvancesCursor(t *testing.T) {
	// DISC-011：取关后旧 inbox 行残留。若本页 limit 行全部属于已取关作者，
	// 仍必须返回 HasMore 并推进游标，否则更早的当前关注作者行被永久跳过。
	inbox := new(mockInboxModel)
	outbox := new(mockOutboxModel)
	userSvc := new(mockUserService)
	contentSvc := new(mockContentService)
	userSvc.On("GetFollowing", mock.Anything, &userservice.GetFollowingReq{UserId: 1, Page: 1, PageSize: followingLookupPageSize}).
		Return(&userservice.GetFollowingResp{Users: followingUsers(8), Total: 1}, nil).Once()
	inbox.On("FindByUserBefore", mock.Anything, int64(1), int64(math.MaxInt64), int64(math.MaxInt64), int64(3)).
		Return([]*model.FeedInbox{
			{UserId: 1, AuthorId: 9, PostId: 300, CreatedAt: 3000},
			{UserId: 1, AuthorId: 9, PostId: 200, CreatedAt: 2000},
			{UserId: 1, AuthorId: 9, PostId: 100, CreatedAt: 1000},
		}, nil).Once()
	outbox.On("FindByAuthorsBefore", mock.Anything, []int64{8}, int64(math.MaxInt64), int64(math.MaxInt64), int64(3)).
		Return([]*model.FeedOutbox{}, nil).Once()

	logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{
		InboxModel: inbox, OutboxModel: outbox, UserService: userSvc, ContentService: contentSvc,
	})
	resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{UserId: 1, PageSize: 2})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Items)
	assert.True(t, resp.HasMore, "full unfollowed page must still allow paging")
	assert.Equal(t, int64(1000), resp.NextCursorCreatedAt, "cursor must advance past scanned rows")
	assert.Equal(t, int64(100), resp.NextCursorPostId)
	contentSvc.AssertNotCalled(t, "GetPostsByIds", mock.Anything, mock.Anything)
	inbox.AssertExpectations(t)
	outbox.AssertExpectations(t)
	userSvc.AssertExpectations(t)
}

func TestGetFollowFeedLogic_UnpublishedCandidatesStillAdvanceCursor(t *testing.T) {
	// 候选行全部未发布（enrich 后不可见）时，本页无可见项但仍有更多行：
	// 必须推进游标避免死路。
	inbox := new(mockInboxModel)
	outbox := new(mockOutboxModel)
	userSvc := new(mockUserService)
	contentSvc := new(mockContentService)
	userSvc.On("GetFollowing", mock.Anything, &userservice.GetFollowingReq{UserId: 1, Page: 1, PageSize: followingLookupPageSize}).
		Return(&userservice.GetFollowingResp{Users: followingUsers(9), Total: 1}, nil).Once()
	inbox.On("FindByUserBefore", mock.Anything, int64(1), int64(math.MaxInt64), int64(math.MaxInt64), int64(3)).Return([]*model.FeedInbox{
		{UserId: 1, AuthorId: 9, PostId: 300, CreatedAt: 3000},
		{UserId: 1, AuthorId: 9, PostId: 200, CreatedAt: 2000},
		{UserId: 1, AuthorId: 9, PostId: 100, CreatedAt: 1000},
	}, nil).Once()
	outbox.On("FindByAuthorsBefore", mock.Anything, []int64{9}, int64(math.MaxInt64), int64(math.MaxInt64), int64(3)).
		Return([]*model.FeedOutbox{}, nil).Once()
	contentSvc.On("GetPostsByIds", mock.Anything, &contentservice.GetPostsByIdsReq{PostIds: []int64{300, 200, 100}}).
		Return(&contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{
			{Id: 300, AuthorId: 9, Status: 2},
			{Id: 200, AuthorId: 9, Status: 0},
			{Id: 100, AuthorId: 9, Status: 2},
		}}, nil).Once()

	logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{
		InboxModel: inbox, OutboxModel: outbox, UserService: userSvc, ContentService: contentSvc,
	})
	resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{UserId: 1, PageSize: 2})

	require.NoError(t, err)
	assert.Empty(t, resp.Items)
	assert.True(t, resp.HasMore)
	assert.Equal(t, int64(1000), resp.NextCursorCreatedAt)
	assert.Equal(t, int64(100), resp.NextCursorPostId)
	inbox.AssertExpectations(t)
	outbox.AssertExpectations(t)
	userSvc.AssertExpectations(t)
	contentSvc.AssertExpectations(t)
}
