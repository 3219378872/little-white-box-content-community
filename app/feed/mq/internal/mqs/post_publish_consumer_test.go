package mqs

import (
	"context"
	"errors"
	"testing"

	"esx/app/feed/mq/internal/model"
	"esx/app/feed/mq/internal/svc"
	"user/userservice"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

// --- fakes ---

type fakeOutboxModel struct{ inserted []*model.FeedOutbox }

func (m *fakeOutboxModel) InsertIgnore(ctx context.Context, row *model.FeedOutbox) error {
	m.inserted = append(m.inserted, row)
	return nil
}

type fakeInboxModel struct{ inserted []*model.FeedInbox }

func (m *fakeInboxModel) BatchInsertIgnore(ctx context.Context, rows []*model.FeedInbox) (int64, error) {
	m.inserted = append(m.inserted, rows...)
	return int64(len(rows)), nil
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

type fakeUserService struct{ followers []*userservice.UserInfo }

func (s *fakeUserService) GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...grpc.CallOption) (*userservice.GetUserResp, error) {
	return &userservice.GetUserResp{User: &userservice.UserInfo{Id: in.UserId, FollowerCount: int64(len(s.followers))}}, nil
}
func (s *fakeUserService) GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...grpc.CallOption) (*userservice.GetFollowersResp, error) {
	return &userservice.GetFollowersResp{Users: s.followers, Total: int64(len(s.followers))}, nil
}

// --- tests ---

func TestPostPublishConsumer_MalformedJSON_Skips(t *testing.T) {
	outbox := &fakeOutboxModel{}
	inbox := &fakeInboxModel{}
	userSvc := &fakeUserService{}
	svcCtx := &svc.ServiceContext{
		OutboxModel: outbox, InboxModel: inbox, UserService: userSvc,
		BigVThreshold: 10000, FanoutBatchSize: 500,
	}

	result := consumeMessageBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`not-json`)}, MsgId: "msg-1"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, outbox.inserted)
}

func TestPostPublishConsumer_MissingFields_Skips(t *testing.T) {
	outbox := &fakeOutboxModel{}
	inbox := &fakeInboxModel{}
	svcCtx := &svc.ServiceContext{
		OutboxModel: outbox, InboxModel: inbox, UserService: &fakeUserService{},
		BigVThreshold: 10000, FanoutBatchSize: 500,
	}

	result := consumeMessageBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"post_id":0}`)}, MsgId: "msg-2"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
}

func TestPostPublishConsumer_UserRPCFailure_Retry(t *testing.T) {
	outbox := &fakeOutboxModel{}
	inbox := &fakeInboxModel{}
	userSvc := new(mockUserService)
	userSvc.On("GetUser", mock.Anything, mock.Anything).
		Return(nil, errors.New("rpc down")).Once()
	svcCtx := &svc.ServiceContext{
		OutboxModel: outbox, InboxModel: inbox, UserService: userSvc,
		BigVThreshold: 10000, FanoutBatchSize: 500,
	}

	result := consumeMessageBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"post_id":1,"author_id":9,"created_at":1710000000000}`)}, MsgId: "msg-3"},
	)

	assert.Equal(t, consumer.ConsumeRetryLater, result)
}

func TestPostPublishConsumer_ValidMessage_Success(t *testing.T) {
	outbox := &fakeOutboxModel{}
	inbox := &fakeInboxModel{}
	userSvc := &fakeUserService{followers: []*userservice.UserInfo{{Id: 1}, {Id: 2}}}
	svcCtx := &svc.ServiceContext{
		OutboxModel: outbox, InboxModel: inbox, UserService: userSvc,
		BigVThreshold: 10000, FanoutBatchSize: 500,
	}

	result := consumeMessageBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"post_id":1,"author_id":9,"created_at":1710000000000}`)}, MsgId: "msg-4"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, outbox.inserted, 1)
	assert.Len(t, inbox.inserted, 2)
}
