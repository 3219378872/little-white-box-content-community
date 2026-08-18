package logic

import (
	"context"
	"encoding/json"
	"errors"
	"esx/pkg/idempotencyx"
	"strconv"
	"strings"
	"testing"

	"errx"
	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/event"
	"esx/pkg/outboxx"
	"mqx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
	_ idempotencyx.IdempotencyRecord,
) (int64, bool, error) {
	c.post = post
	c.tags = append([]string(nil), tags...)
	c.event = event
	return post.Id, c.result == nil, c.result
}

func (c *capturingPostCommand) UpdatePost(_ context.Context, _ int64, _ map[string]any, _ []string, _ []int64, event outboxx.Event, _ int64) error {
	c.event = event
	return nil
}

func (c *capturingPostCommand) DeletePost(_ context.Context, _ int64, event outboxx.Event, _ int64) error {
	c.event = event
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
	assert.Equal(t, int64(1), payload.Revision)
	assert.Equal(t, strconv.FormatInt(resp.PostId, 10), command.event.Key)
}

func TestUpdatePostLogicWritesRevisionAndPostKeyedOutbox(t *testing.T) {
	postModel := new(MockPostModel)
	postModel.On("FindPostById", mock.Anything, int64(300)).Return(&model.Post{
		Id: 300, AuthorId: 3001, Title: "old", Content: "old", Status: 1, Revision: 3,
	}, nil)
	command := &capturingPostCommand{}
	svcCtx := newUnitSvcCtx(postModel, nil, nil, new(MockPostTagModel))
	svcCtx.PostCommandModel = command

	_, err := NewUpdatePostLogic(context.Background(), svcCtx).UpdatePost(&pb.UpdatePostReq{
		PostId: 300, AuthorId: 3001, Title: "C", Content: "C-body", ExpectedRevision: 3,
	})
	require.NoError(t, err)
	assert.Equal(t, mqx.TopicPostUpdate, command.event.Topic)
	assert.Equal(t, "300", command.event.Key)

	var payload event.PostEvent
	require.NoError(t, json.Unmarshal(command.event.Payload, &payload))
	assert.Equal(t, event.PostEventUpdated, payload.Type)
	assert.Equal(t, int64(300), payload.PostID)
	assert.Equal(t, int64(4), payload.Revision)
	assert.Equal(t, "C", payload.Title)
}

func TestDeletePostLogicWritesRevisionAndPostKeyedOutbox(t *testing.T) {
	postModel := new(MockPostModel)
	postModel.On("FindPostById", mock.Anything, int64(300)).Return(&model.Post{
		Id: 300, AuthorId: 3001, Status: 1, Revision: 4,
	}, nil)
	command := &capturingPostCommand{}
	svcCtx := newUnitSvcCtx(postModel, nil, nil, new(MockPostTagModel))
	svcCtx.PostCommandModel = command

	_, err := NewDeletePostLogic(context.Background(), svcCtx).DeletePost(&pb.DeletePostReq{
		PostId: 300, AuthorId: 3001, ExpectedRevision: 4,
	})
	require.NoError(t, err)
	assert.Equal(t, mqx.TopicPostDelete, command.event.Topic)
	assert.Equal(t, "300", command.event.Key)

	var payload event.PostEvent
	require.NoError(t, json.Unmarshal(command.event.Payload, &payload))
	assert.Equal(t, event.PostEventDeleted, payload.Type)
	assert.Equal(t, int64(300), payload.PostID)
	assert.Equal(t, int64(5), payload.Revision)
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

func TestCreatePostLogicMapsIdempotencyConflict(t *testing.T) {
	command := &capturingPostCommand{result: idempotencyx.ErrIdempotencyConflict}
	svcCtx := newUnitSvcCtx(new(MockPostModel), nil, nil, new(MockPostTagModel))
	svcCtx.PostCommandModel = command

	resp, err := NewCreatePostLogic(context.Background(), svcCtx).CreatePost(&pb.CreatePostReq{
		AuthorId: 9, Title: "title", Content: "content", IdempotencyKey: "key-1",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.IdempotencyConflict), "期望幂等冲突码，实际: %v", err)
}

func TestCreatePostLogicIdempotentRetryReturnsOriginalPostID(t *testing.T) {
	command := &capturingPostCommand{}
	svcCtx := newUnitSvcCtx(new(MockPostModel), nil, nil, new(MockPostTagModel))
	svcCtx.PostCommandModel = command

	resp, err := NewCreatePostLogic(context.Background(), svcCtx).CreatePost(&pb.CreatePostReq{
		AuthorId: 9, Title: "title", Content: "content", IdempotencyKey: "key-1",
	})

	require.NoError(t, err)
	require.NotNil(t, command.post)
	assert.Equal(t, command.post.Id, resp.PostId)
	assert.Equal(t, int64(1), resp.Revision)
}

func TestCreatePostLogicRejectsOversizedTitle(t *testing.T) {
	svcCtx := newUnitSvcCtx(new(MockPostModel), nil, nil, new(MockPostTagModel))
	resp, err := NewCreatePostLogic(context.Background(), svcCtx).CreatePost(&pb.CreatePostReq{
		AuthorId: 9, Title: strings.Repeat("长", 121), Content: "content",
	})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ContentTooLong), "期望内容过长码，实际: %v", err)
}

func TestCreatePostLogicRejectsTooManyImagesAndTags(t *testing.T) {
	images := make([]string, 10)
	for i := range images {
		images[i] = "http://example.com/img"
	}
	svcCtx := newUnitSvcCtx(new(MockPostModel), nil, nil, new(MockPostTagModel))
	resp, err := NewCreatePostLogic(context.Background(), svcCtx).CreatePost(&pb.CreatePostReq{
		AuthorId: 9, Title: "title", Content: "content", Images: images,
	})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ParamError))

	tags := make([]string, 11)
	for i := range tags {
		tags[i] = "tag"
	}
	resp, err = NewCreatePostLogic(context.Background(), svcCtx).CreatePost(&pb.CreatePostReq{
		AuthorId: 9, Title: "title", Content: "content", Tags: tags,
	})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ParamError))
}

func TestCreatePostLogicRejectsOversizedContentAndTag(t *testing.T) {
	svcCtx := newUnitSvcCtx(new(MockPostModel), nil, nil, new(MockPostTagModel))

	// CORE-020：正文上限 20,000 Unicode 字符。
	resp, err := NewCreatePostLogic(context.Background(), svcCtx).CreatePost(&pb.CreatePostReq{
		AuthorId: 9, Title: "title", Content: strings.Repeat("长", 20001),
	})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ContentTooLong), "期望内容过长码，实际: %v", err)

	// CORE-021：标签上限 32 Unicode 字符。
	resp, err = NewCreatePostLogic(context.Background(), svcCtx).CreatePost(&pb.CreatePostReq{
		AuthorId: 9, Title: "title", Content: "content", Tags: []string{strings.Repeat("长", 33)},
	})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ParamError), "期望参数错误码，实际: %v", err)

	// 边界值 20,000 与 32 应通过。
	pm := new(MockPostModel)
	ptm := new(MockPostTagModel)
	pm.On("InsertPostTx", mock.Anything, mock.Anything, mock.AnythingOfType("*model.Post")).Return(nil)
	ptm.On("BatchInsertTagsByPostIdTx", mock.Anything, mock.Anything, mock.AnythingOfType("int64"), mock.Anything, mock.Anything).Return(nil)
	boundarySvcCtx := newUnitSvcCtx(pm, nil, nil, ptm)
	resp, err = NewCreatePostLogic(context.Background(), boundarySvcCtx).CreatePost(&pb.CreatePostReq{
		AuthorId: 9, Title: "title", Content: strings.Repeat("长", 20000), Tags: []string{strings.Repeat("长", 32)},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestCreatePostCommandHashCoversStatusAndMediaIDs(t *testing.T) {
	// CORE-050/051：幂等命令哈希必须区分 status 与媒体引用；
	// 同键但命令不同必须产生冲突，而不是静默返回旧资源。
	base := &pb.CreatePostReq{
		AuthorId: 1, Title: "title", Content: "content",
		Tags: []string{"go"}, MediaIds: []int64{3, 1}, Status: 1,
		IdempotencyKey: "key-1",
	}
	baseHash := postCommandHashForTest(base)

	draft := &pb.CreatePostReq{
		AuthorId: 1, Title: "title", Content: "content",
		Tags: []string{"go"}, MediaIds: []int64{3, 1}, Status: 0,
		IdempotencyKey: "key-1",
	}
	if postCommandHashForTest(draft) == baseHash {
		t.Fatal("status change must change the idempotency command hash")
	}

	reordered := &pb.CreatePostReq{
		AuthorId: 1, Title: "title", Content: "content",
		Tags: []string{"go"}, MediaIds: []int64{1, 3}, Status: 1,
		IdempotencyKey: "key-1",
	}
	if postCommandHashForTest(reordered) != baseHash {
		t.Fatal("media id order must not change the idempotency command hash")
	}

	withoutMedia := &pb.CreatePostReq{
		AuthorId: 1, Title: "title", Content: "content",
		Tags: []string{"go"}, Status: 1,
		IdempotencyKey: "key-1",
	}
	if postCommandHashForTest(withoutMedia) == baseHash {
		t.Fatal("media id set change must change the idempotency command hash")
	}
}

func postCommandHashForTest(in *pb.CreatePostReq) string {
	return idempotencyx.CommandHash(
		in.GetTitle(), in.GetContent(), strings.Join(in.Images, ","), strings.Join(in.Tags, ","),
		strings.Join(sortedMediaIDs(in.MediaIds), ","), strconv.FormatInt(int64(in.GetStatus()), 10),
	)
}
