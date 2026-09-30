package logic

import (
	"context"
	"esx/app/content/rpc/internal/model"
	pb "esx/kitex_gen/content"
	"esx/pkg/errx"
	"esx/pkg/idempotencyx"
	"esx/pkg/outboxx"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type captureTagsCommand struct {
	capturingPostCommand
	replace  bool
	calls    int
	hash     string
	revision int64
}

func (c *captureTagsCommand) UpdatePost(_ context.Context, _ int64, _ map[string]any, tags []string, _ []int64, _ outboxx.Event, _ int64, replace bool) error {
	c.calls++
	c.replace = replace
	c.tags = tags
	return nil
}
func (c *captureTagsCommand) ReplayPostCommand(_ context.Context, rec idempotencyx.IdempotencyRecord) (int64, bool, error) {
	if c.hash == "" {
		return 0, false, nil
	}
	if !rec.Matches(c.hash) {
		return 0, false, idempotencyx.ErrIdempotencyConflict
	}
	return c.revision, true, nil
}
func (c *captureTagsCommand) UpdatePostIdempotent(ctx context.Context, id int64, fields map[string]any, tags []string, ids []int64, event outboxx.Event, revision int64, replace bool, result int64, rec idempotencyx.IdempotencyRecord) (bool, error) {
	c.hash, c.revision = rec.CommandHash, result
	return true, c.UpdatePost(ctx, id, fields, tags, ids, event, revision, replace)
}
func (c *captureTagsCommand) DeletePostIdempotent(context.Context, int64, outboxx.Event, int64, int64, idempotencyx.IdempotencyRecord) (bool, error) {
	panic("unexpected delete")
}

func TestUpdateTagsPresenceAcrossRPC(t *testing.T) {
	for _, tt := range []struct {
		name, title       string
		tags              []string
		provided, replace bool
	}{
		{name: "omitted", title: "new title"},
		{name: "empty with title", title: "new title", tags: []string{}, provided: true, replace: true},
		{name: "empty only", tags: []string{}, provided: true, replace: true},
		{name: "nonempty", tags: []string{"new"}, provided: true, replace: true},
		{name: "old client nonempty", tags: []string{"new"}, replace: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := &pb.UpdatePostReq{PostId: 5, AuthorId: 1, ExpectedRevision: 3, Title: tt.title, Tags: tt.tags, TagsProvided: tt.provided, IdempotencyKey: "same-key"}
			wire, err := proto.Marshal(input)
			require.NoError(t, err)
			req := new(pb.UpdatePostReq)
			require.NoError(t, proto.Unmarshal(wire, req))
			posts, tags := new(MockPostModel), new(MockPostTagModel)
			posts.On("FindPostById", mock.Anything, int64(5)).Return(&model.Post{Id: 5, AuthorId: 1, Title: "old", Content: "body", Status: 1, Revision: 3}, nil).Once()
			if !tt.replace {
				tags.On("FindTagNamesByPostId", mock.Anything, int64(5)).Return([]string{"old"}, nil).Once()
			}
			cmd := new(captureTagsCommand)
			svc := newUnitSvcCtx(posts, nil, nil, tags)
			svc.PostCommandModel = cmd
			logic := NewUpdatePostLogic(context.Background(), svc)
			resp, err := logic.UpdatePost(req)
			require.NoError(t, err)
			require.Equal(t, int64(4), resp.Revision)
			require.Equal(t, tt.replace, cmd.replace)
			require.Equal(t, 1, cmd.calls)
			if tt.replace {
				require.Equal(t, len(tt.tags), len(cmd.tags))
				require.Equal(t, tt.tags, append([]string{}, cmd.tags...))
			}
			replay, err := logic.UpdatePost(req)
			require.NoError(t, err)
			require.Equal(t, resp.Revision, replay.Revision)
			require.Equal(t, 1, cmd.calls)
			if tt.provided && len(tt.tags) == 0 {
				req.TagsProvided = false
				req.Title = "new title"
				_, err = logic.UpdatePost(req)
				require.True(t, errx.Is(err, errx.IdempotencyConflict))
			}
			posts.AssertExpectations(t)
			tags.AssertExpectations(t)
		})
	}
}
