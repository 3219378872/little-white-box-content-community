package logic

import (
	"context"
	"errx"
	"esx/app/content/internal/model"
	"esx/app/content/pb/xiaobaihe/content/pb"
	"fmt"

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
		if err == model.ErrNotFound {
			return nil, errx.NewWithCode(errx.ContentNotFound)
		}
		return nil, fmt.Errorf("查询评论失败: %w", err)
	}

	if comment.Status == 0 {
		return &pb.DeleteCommentResp{}, nil // 已删除，幂等返回
	}
	if comment.UserId != in.UserId {
		return nil, errx.NewWithCode(errx.ContentForbidden)
	}

	if err = l.svcCtx.CommentModel.UpdateStatus(l.ctx, comment.Id, 0); err != nil {
		return nil, fmt.Errorf("删除评论失败: %w", err)
	}

	// 原子递减评论数，避免并发更新丢失
	if err = l.svcCtx.PostModel.DecrCommentCount(l.ctx, comment.PostId); err != nil {
		l.Logger.Errorf("更新评论数失败 postId=%d err=%v", comment.PostId, err)
	}

	return &pb.DeleteCommentResp{}, nil
}
