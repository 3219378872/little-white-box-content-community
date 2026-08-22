package logic

import (
	"context"
	"errors"
	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/errx"

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

	if l.svcCtx.CommentCommandModel == nil {
		l.Errorw("CommentCommandModel is nil")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err = l.svcCtx.CommentCommandModel.DeleteComment(l.ctx, comment); err != nil {
		l.Errorw("delete comment transaction failed",
			logx.Field("commentId", comment.Id),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err = l.svcCtx.CommentModel.InvalidateCommentCache(l.ctx, comment.Id); err != nil {
		l.Errorw("invalidate comment cache after delete failed", logx.Field("err", err.Error()))
	}
	if err = l.svcCtx.PostModel.InvalidatePostCache(l.ctx, comment.PostId); err != nil {
		l.Errorw("invalidate post cache after comment delete failed", logx.Field("err", err.Error()))
	}

	return &pb.DeleteCommentResp{}, nil
}
