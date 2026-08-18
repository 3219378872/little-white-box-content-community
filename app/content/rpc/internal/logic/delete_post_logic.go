package logic

import (
	"context"
	"errors"
	"errx"
	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/event"
	"esx/pkg/visibilityx"
	"mqx"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeletePostLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDeletePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeletePostLogic {
	return &DeletePostLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// DeletePost 删除帖子（软删除，status=2）
func (l *DeletePostLogic) DeletePost(in *pb.DeletePostReq) (*pb.DeletePostResp, error) {
	if in.PostId <= 0 || in.AuthorId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	post, err := l.svcCtx.PostModel.FindPostById(l.ctx, in.PostId)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.NewWithCode(errx.ContentNotFound)
		}
		l.Errorw("PostModel.FindPostById failed",
			logx.Field("postId", in.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if visibilityx.IsDeleted(int32(post.Status)) {
		return nil, errx.NewWithCode(errx.PostAlreadyDeleted)
	}
	if post.AuthorId != in.AuthorId {
		return nil, errx.NewWithCode(errx.ContentForbidden)
	}
	// CORE-013/062：迁移期允许 expected_revision=0（旧客户端），不做版本检查。
	if in.ExpectedRevision > 0 && post.Revision != in.ExpectedRevision {
		return nil, errx.NewWithCode(errx.ContentVersionConflict)
	}

	outboxEvent, err := buildPostOutboxEvent(mqx.TopicPostDelete, event.PostEvent{
		Type:     event.PostEventDeleted,
		PostID:   post.Id,
		AuthorID: post.AuthorId,
		Revision: post.Revision + 1,
	})
	if err != nil {
		l.Errorw("build post-deleted event failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if l.svcCtx.PostCommandModel == nil {
		l.Errorw("PostCommandModel is nil")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err = l.svcCtx.PostCommandModel.DeletePost(l.ctx, post.Id, outboxEvent, in.ExpectedRevision); err != nil {
		if errors.Is(err, model.ErrVersionConflict) {
			return nil, errx.NewWithCode(errx.ContentVersionConflict)
		}
		l.Errorw("delete post transaction failed",
			logx.Field("postId", post.Id), logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err = l.svcCtx.PostModel.InvalidatePostCache(l.ctx, post.Id); err != nil {
		l.Errorw("invalidate post cache after delete failed",
			logx.Field("postId", post.Id), logx.Field("err", err.Error()))
	}

	return &pb.DeletePostResp{}, nil
}
