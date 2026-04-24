package logic

import (
	"context"
	"encoding/json"
	"testing"

	"esx/app/content/pb/xiaobaihe/content/pb"
	"mqx"

	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

func TestCreatePostLogic_PublishPostCreatedEvent(t *testing.T) {
	mq := &fakeMQProducer{}
	pm := new(MockPostModel)
	ptm := new(MockPostTagModel)
	pm.On("InsertPost", mock.Anything, mock.AnythingOfType("*model.Post")).Return(nil)
	ptm.On("BatchInsertTagsByPostId", mock.Anything, mock.Anything, mock.AnythingOfType("int64"), mock.Anything, mock.Anything).Return(nil)
	svcCtx := newUnitSvcCtx(pm, nil, nil, ptm)
	svcCtx.MQProducer = mq
	logic := NewCreatePostLogic(context.Background(), svcCtx)
	resp, err := logic.CreatePost(&pb.CreatePostReq{AuthorId: 9, Title: "t", Content: "content"})

	require.NoError(t, err)
	require.NotZero(t, resp.PostId)
	assert.Equal(t, mqx.TopicPostCreate, mq.topic)
	assert.Equal(t, mqx.TagDefault, mq.tag)
	var got map[string]int64
	require.NoError(t, json.Unmarshal(mq.body, &got))
	assert.Equal(t, int64(9), got["author_id"])
	assert.Equal(t, resp.PostId, got["post_id"])
	assert.NotZero(t, got["created_at"])
}
