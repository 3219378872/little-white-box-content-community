package logic

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"esx/app/content/rpc/internal/model"
	pb "esx/kitex_gen/content"
	"esx/pkg/errx"
	"esx/pkg/idempotencyx"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestReplyTargetRequiresVisibleSameRootAuthor(t *testing.T) {
	for _, tt := range []struct {
		name           string
		target         *model.Comment
		lookupErr      error
		parent         *model.Comment
		user, targetID int64
		code           int
	}{
		{name: "child author", target: &model.Comment{Id: 56, PostId: 1, ParentId: sql.NullInt64{Int64: 55, Valid: true}, UserId: 4, Status: 1}, user: 4, targetID: 56},
		{name: "explicit root", user: 3, targetID: 55},
		{name: "legacy root", user: 3},
		{name: "forged child author", target: &model.Comment{Id: 56, PostId: 1, ParentId: sql.NullInt64{Int64: 55, Valid: true}, UserId: 4, Status: 1}, user: 99, targetID: 56, code: errx.ParamError},
		{name: "other root child", target: &model.Comment{Id: 56, PostId: 1, ParentId: sql.NullInt64{Int64: 99, Valid: true}, UserId: 4, Status: 1}, user: 4, targetID: 56, code: errx.ParamError},
		{name: "other post child", target: &model.Comment{Id: 56, PostId: 2, ParentId: sql.NullInt64{Int64: 55, Valid: true}, UserId: 4, Status: 1}, user: 4, targetID: 56, code: errx.ParamError},
		{name: "deleted child", target: &model.Comment{Id: 56, PostId: 1, ParentId: sql.NullInt64{Int64: 55, Valid: true}, UserId: 4, Status: 2}, user: 4, targetID: 56, code: errx.ContentNotFound},
		{name: "missing child", lookupErr: model.ErrNotFound, user: 4, targetID: 56, code: errx.ParamError},
		{name: "child lookup failure", lookupErr: fmt.Errorf("database unavailable"), user: 4, targetID: 56, code: errx.SystemError},
		{name: "deleted root", parent: &model.Comment{Id: 55, PostId: 1, UserId: 3, Status: 2}, user: 4, targetID: 56, code: errx.ContentNotFound},
		{name: "nested parent", parent: &model.Comment{Id: 55, PostId: 1, UserId: 3, Status: 1, ParentId: sql.NullInt64{Int64: 54, Valid: true}}, user: 4, targetID: 56, code: errx.ParamError},
		{name: "negative target", user: 3, targetID: -1, code: errx.ParamError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			post, comments := new(MockPostModel), new(MockCommentModel)
			post.On("FindPostById", mock.Anything, int64(1)).Return(&model.Post{Id: 1, Status: 1}, nil)
			if tt.targetID >= 0 {
				parent := tt.parent
				if parent == nil {
					parent = &model.Comment{Id: 55, PostId: 1, UserId: 3, Status: 1}
				}
				comments.On("FindCommentById", mock.Anything, int64(55)).Return(parent, nil)
			}
			if tt.target != nil || tt.lookupErr != nil {
				comments.On("FindCommentById", mock.Anything, tt.targetID).Return(tt.target, tt.lookupErr)
			}
			logic := NewCreateCommentLogic(context.Background(), newUnitSvcCtx(post, comments, nil, nil))
			err := logic.validateCommentTarget(&pb.CreateCommentReq{PostId: 1, UserId: 9, ParentId: 55, ReplyUserId: tt.user, ReplyToCommentId: tt.targetID})
			if tt.code == 0 {
				require.NoError(t, err)
			} else {
				require.True(t, errx.Is(err, tt.code), "error=%v", err)
			}
			post.AssertExpectations(t)
			comments.AssertExpectations(t)
		})
	}
}

func TestCommentFingerprintPreservesRootRetryAndBindsChild(t *testing.T) {
	req := &pb.CreateCommentReq{PostId: 1, UserId: 9, ParentId: 55, ReplyUserId: 3, Content: "reply", IdempotencyKey: "same-key"}
	legacy := idempotencyx.CommandHash("reply", "1", "55", "3")
	require.Equal(t, legacy, commentIdempotencyRecord(req).CommandHash)
	req.ReplyToCommentId = 55
	require.Equal(t, legacy, commentIdempotencyRecord(req).CommandHash)
	req.ReplyToCommentId = 56
	child := commentIdempotencyRecord(req).CommandHash
	require.NotEqual(t, legacy, child)
	req.ReplyToCommentId = 57
	require.NotEqual(t, child, commentIdempotencyRecord(req).CommandHash)
}

func TestCreateCommentPersistsFlattenedChildReply(t *testing.T) {
	posts, comments := new(MockPostModel), new(MockCommentModel)
	posts.On("FindPostById", mock.Anything, int64(1)).Return(&model.Post{Id: 1, Status: 1}, nil)
	comments.On("FindCommentById", mock.Anything, int64(55)).Return(&model.Comment{Id: 55, PostId: 1, UserId: 3, Status: 1}, nil)
	comments.On("FindCommentById", mock.Anything, int64(56)).Return(&model.Comment{Id: 56, PostId: 1, UserId: 4, Status: 1, ParentId: sql.NullInt64{Int64: 55, Valid: true}}, nil)
	comments.On("InsertComment", mock.Anything, mock.MatchedBy(func(c *model.Comment) bool {
		return c.ParentId.Valid && c.ParentId.Int64 == 55 && c.ReplyUserId.Valid && c.ReplyUserId.Int64 == 4 && c.UserId == 9
	})).Return(nil)
	posts.On("IncrCommentCount", mock.Anything, int64(1)).Return(nil)
	reply, err := NewCreateCommentLogic(context.Background(), newUnitSvcCtx(posts, comments, nil, nil)).CreateComment(&pb.CreateCommentReq{
		PostId: 1, UserId: 9, ParentId: 55, ReplyToCommentId: 56, ReplyUserId: 4, Content: "child reply", IdempotencyKey: "key",
	})
	require.NoError(t, err)
	require.Positive(t, reply.CommentId)
	posts.AssertExpectations(t)
	comments.AssertExpectations(t)
}
