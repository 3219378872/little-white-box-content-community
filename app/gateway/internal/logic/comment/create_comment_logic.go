// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package comment

import (
	"context"

	"errx"
	"esx/app/content/contentservice"
	"jwtx"

	"gateway/internal/svc"
	"gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateCommentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 创建评论
func NewCreateCommentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateCommentLogic {
	return &CreateCommentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateCommentLogic) CreateComment(req *types.CreateCommentReq) (resp *types.CreateCommentResp, err error) {
	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	result, err := l.svcCtx.ContentService.CreateComment(l.ctx, &contentservice.CreateCommentReq{
		PostId:      req.PostId,
		UserId:      userId,
		ParentId:    req.ParentId,
		ReplyUserId: req.ReplyUserId,
		Content:     req.Content,
	})
	if err != nil {
		l.Errorw("ContentService.CreateComment RPC failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.CreateCommentResp{
		CommentId: result.CommentId,
	}, nil
}
