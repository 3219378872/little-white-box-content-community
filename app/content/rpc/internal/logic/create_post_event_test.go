package logic

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/event"
	"esx/pkg/outboxx"
	"mqx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturingPostCommand struct {
	post   *model.Post
	tags   []string
	event  outboxx.Event
	result error
}

func (c *capturingPostCommand) CreatePost(
	_ context.Context,
	post *model.Post,
	tags []string,
	_ []int64,
	event outboxx.Event,
) error {
	c.post = post
	c.tags = append([]string(nil), tags...)
	c.event = event
	return c.result
}

func (*capturingPostCommand) UpdatePost(context.Context, int64, map[string]any, []string, []int64, outboxx.Event) error {
	return nil
}

func (*capturingPostCommand) DeletePost(context.Context, int64, outboxx.Event) error {
	return nil
}

func TestCreatePostLogicWritesLifecycleEventInCommandTransaction(t *testing.T) {
	postModel := new(MockPostModel)
	command := &capturingPostCommand{}
	svcCtx := newUnitSvcCtx(postModel, nil, nil, new(MockPostTagModel))
	svcCtx.PostCommandModel = command
	logic := NewCreatePostLogic(context.Background(), svcCtx)

	resp, err := logic.CreatePost(&pb.CreatePostReq{
		AuthorId: 9, Title: "title", Content: "content", Tags: []string{"x"},
	})

	require.NoError(t, err)
	require.NotZero(t, resp.PostId)
	require.NotNil(t, command.post)
	assert.Equal(t, resp.PostId, command.post.Id)
	assert.Equal(t, []string{"x"}, command.tags)
	assert.Equal(t, mqx.TopicPostCreate, command.event.Topic)

	var payload event.PostEvent
	require.NoError(t, json.Unmarshal(command.event.Payload, &payload))
	assert.Equal(t, event.PostEventCreated, payload.Type)
	assert.Equal(t, resp.PostId, payload.PostID)
	assert.Equal(t, int64(9), payload.AuthorID)
}

func TestCreatePostLogicRejectsWhenTransactionalOutboxWriteFails(t *testing.T) {
	command := &capturingPostCommand{result: errors.New("outbox insert failed")}
	svcCtx := newUnitSvcCtx(new(MockPostModel), nil, nil, new(MockPostTagModel))
	svcCtx.PostCommandModel = command

	resp, err := NewCreatePostLogic(context.Background(), svcCtx).CreatePost(&pb.CreatePostReq{
		AuthorId: 9, Title: "title", Content: "content",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
}
