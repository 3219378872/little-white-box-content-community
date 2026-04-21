package logic

import (
	"context"
	"errors"
	"errx"
	"esx/app/content/internal/model"
	"esx/app/content/pb/xiaobaihe/content/pb"

	"esx/app/content/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteCommentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDeleteCommentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteCommentLogic {
	return &DeleteCommentLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// DeleteComment 删除评论（软删除，status=0）
func (l *DeleteCommentLogic) DeleteComment(in *pb.DeleteCommentReq) (*pb.DeleteCommentResp, error) {
	if in.CommentId <= 0 || in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	comment, err := l.svcCtx.CommentModel.FindCommentById(l.ctx, in.CommentId)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.NewWithCode(errx.ContentNotFound)
		}
		l.Errorw("CommentModel.FindCommentById failed",
			logx.Field("commentId", in.CommentId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if comment.Status == 0 {
		return &pb.DeleteCommentResp{}, nil // 已删除，幂等返回
	}
	if comment.UserId != in.UserId {
		return nil, errx.NewWithCode(errx.ContentForbidden)
	}

	if err = l.svcCtx.CommentModel.UpdateStatus(l.ctx, comment.Id, 0); err != nil {
		l.Errorw("CommentModel.UpdateStatus failed",
			logx.Field("commentId", comment.Id),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	// 原子递减评论数，避免并发更新丢失
	if err = l.svcCtx.PostModel.DecrCommentCount(l.ctx, comment.PostId); err != nil {
		l.Errorw("PostModel.DecrCommentCount failed",
			logx.Field("postId", comment.PostId),
			logx.Field("err", err.Error()),
		)
	}

	return &pb.DeleteCommentResp{}, nil
}
