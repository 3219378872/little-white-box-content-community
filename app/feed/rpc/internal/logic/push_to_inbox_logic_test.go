package logic

import (
	"context"
	"errors"
	"testing"

	"errx"
	"esx/app/feed/internal/model"
	"esx/app/feed/internal/svc"
	"esx/app/feed/xiaobaihe/feed/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

func TestPushToInboxLogic_Success(t *testing.T) {
	inbox := new(mockInboxModel)
	inbox.On("BatchInsertIgnore", mock.Anything, mock.MatchedBy(func(rows []*model.FeedInbox) bool {
		return len(rows) == 2 && rows[0].UserId == 1 && rows[1].UserId == 2 && rows[0].AuthorId == 9 && rows[0].PostId == 1001
	})).Return(int64(2), nil).Once()

	logic := NewPushToInboxLogic(context.Background(), &svc.ServiceContext{InboxModel: inbox})
	resp, err := logic.PushToInbox(&pb.PushToInboxReq{AuthorId: 9, PostId: 1001, CreatedAt: 1710000000000, FollowerIds: []int64{1, 2}})

	require.NoError(t, err)
	assert.Equal(t, int64(2), resp.PushedCount)
	inbox.AssertExpectations(t)
}

func TestPushToInboxLogic_EmptyFollowers(t *testing.T) {
	logic := NewPushToInboxLogic(context.Background(), &svc.ServiceContext{})
	resp, err := logic.PushToInbox(&pb.PushToInboxReq{AuthorId: 9, PostId: 1001, CreatedAt: 1710000000000})

	require.NoError(t, err)
	assert.Equal(t, int64(0), resp.PushedCount)
}

func TestPushToInboxLogic_InvalidInput(t *testing.T) {
	logic := NewPushToInboxLogic(context.Background(), &svc.ServiceContext{})
	resp, err := logic.PushToInbox(&pb.PushToInboxReq{AuthorId: 0, PostId: 1001, CreatedAt: 1710000000000})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, errx.ParamError, errx.GetCode(err))
}

func TestPushToInboxLogic_ModelError(t *testing.T) {
	inbox := new(mockInboxModel)
	inbox.On("BatchInsertIgnore", mock.Anything, mock.Anything).Return(int64(0), errors.New("db down")).Once()

	logic := NewPushToInboxLogic(context.Background(), &svc.ServiceContext{InboxModel: inbox})
	resp, err := logic.PushToInbox(&pb.PushToInboxReq{AuthorId: 9, PostId: 1001, CreatedAt: 1710000000000, FollowerIds: []int64{1}})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
	inbox.AssertExpectations(t)
}
