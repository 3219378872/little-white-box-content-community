package logic

import (
	"context"
	"database/sql"
	"encoding/json"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	feedpb "esx/app/feed/rpc/xiaobaihe/feed/pb"
	"esx/pkg/event"
	"testing"

	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type fakeMQProducer struct {
	topic string
	tag   string
	body  []byte
}

func (f *fakeMQProducer) SendSyncWithTag(ctx context.Context, topic, tag string, body []byte) (*primitive.SendResult, error) {
	f.topic = topic
	f.tag = tag
	f.body = append([]byte(nil), body...)
	return &primitive.SendResult{}, nil
}

type fakePostCreateMsg struct {
	actions       []string
	payloads      []proto.Message
	queryPrepared string
	didSubmit     bool
}

func (m *fakePostCreateMsg) Add(action string, payload proto.Message) {
	m.actions = append(m.actions, action)
	m.payloads = append(m.payloads, payload)
}

func (m *fakePostCreateMsg) DoAndSubmitDB(queryPrepared string, fn func(*sql.Tx) error) error {
	m.didSubmit = true
	m.queryPrepared = queryPrepared
	return fn(nil)
}

type fakePostCreateMsgFactory struct {
	msg *fakePostCreateMsg
}

func (f fakePostCreateMsgFactory) NewGID() string {
	return "gid-create-post"
}

func (f fakePostCreateMsgFactory) NewPostCreateMsg(gid string) svc.PostCreateMsg {
	return f.msg
}

func TestCreatePostLogic_UsesDTMFeedFanoutBranch(t *testing.T) {
	msg := &fakePostCreateMsg{}
	pm := new(MockPostModel)
	ptm := new(MockPostTagModel)
	pm.On("InsertPostTx", mock.Anything, mock.Anything, mock.AnythingOfType("*model.Post")).Return(nil).Once()
	ptm.On("BatchInsertTagsByPostIdTx", mock.Anything, mock.Anything, mock.AnythingOfType("int64"), mock.Anything, mock.Anything).Return(nil).Once()
	svcCtx := newUnitSvcCtx(pm, nil, nil, ptm)
	svcCtx.Config.FeedBusiServer = "feed:9091"
	svcCtx.Config.ContentBusiServer = "content:8088"
	svcCtx.PostCreateMsgFactory = fakePostCreateMsgFactory{msg: msg}
	logic := NewCreatePostLogic(context.Background(), svcCtx)

	resp, err := logic.CreatePost(&pb.CreatePostReq{AuthorId: 9, Title: "t", Content: "content"})

	require.NoError(t, err)
	require.NotZero(t, resp.PostId)
	require.True(t, msg.didSubmit)
	assert.Equal(t, "content:8088/content.ContentService/QueryPrepared", msg.queryPrepared)
	require.Equal(t, []string{"feed:9091/feed.FeedService/FanoutPost"}, msg.actions)
	require.Len(t, msg.payloads, 1)
	payload, ok := msg.payloads[0].(*feedpb.FanoutPostReq)
	require.True(t, ok)
	assert.Equal(t, int64(9), payload.AuthorId)
	assert.Equal(t, resp.PostId, payload.PostId)
	assert.NotZero(t, payload.CreatedAt)
	pm.AssertExpectations(t)
	ptm.AssertExpectations(t)
}

// 父 spec §2 要求业务服务不直接写 ES/Milvus/ClickHouse，只发 MQ 消息。
// CreatePostLogic 在 DTM 提交成功后，必须额外发 post-create 事件给
// search/embedding/cleanup/feed-fanout 等 L1 消费者。
func TestCreatePostLogic_PublishesPostCreatedEvent(t *testing.T) {
	msg := &fakePostCreateMsg{}
	mq := &fakeMQProducer{}
	pm := new(MockPostModel)
	ptm := new(MockPostTagModel)
	pm.On("InsertPostTx", mock.Anything, mock.Anything, mock.AnythingOfType("*model.Post")).Return(nil).Once()
	ptm.On("BatchInsertTagsByPostIdTx", mock.Anything, mock.Anything, mock.AnythingOfType("int64"), mock.Anything, mock.Anything).Return(nil).Once()
	svcCtx := newUnitSvcCtx(pm, nil, nil, ptm)
	svcCtx.Config.FeedBusiServer = "feed:9091"
	svcCtx.Config.ContentBusiServer = "content:8088"
	svcCtx.PostCreateMsgFactory = fakePostCreateMsgFactory{msg: msg}
	svcCtx.MQProducer = mq
	logic := NewCreatePostLogic(context.Background(), svcCtx)

	resp, err := logic.CreatePost(&pb.CreatePostReq{AuthorId: 9, Title: "t", Content: "content", Tags: []string{"x"}})

	require.NoError(t, err)
	require.NotZero(t, resp.PostId)
	assert.Equal(t, "post-create", mq.topic)
	assert.Equal(t, "default", mq.tag)
	require.NotNil(t, mq.body)

	var got event.PostEvent
	require.NoError(t, json.Unmarshal(mq.body, &got))
	assert.Equal(t, event.PostEventCreated, got.Type)
	assert.Equal(t, resp.PostId, got.PostID)
	assert.Equal(t, int64(9), got.AuthorID)
	assert.Equal(t, "t", got.Title)
	assert.Equal(t, []string{"x"}, got.Tags)
	pm.AssertExpectations(t)
	ptm.AssertExpectations(t)
}
