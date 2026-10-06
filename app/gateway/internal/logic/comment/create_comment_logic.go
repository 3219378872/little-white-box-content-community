package comment

import (
	"context"

	"esx/app/content/rpc/contentservice"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"

	"esx/pkg/logging"
)

type CreateCommentLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 创建评论
func NewCreateCommentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateCommentLogic {
	return &CreateCommentLogic{
		Logger: logging.WithContext(ctx),
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
		PostId:         req.PostId,
		UserId:         userId,
		ParentId:       req.ParentId,
		ReplyUserId:    req.ReplyUserId,
		Content:        req.Content,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		l.Errorw("ContentService.CreateComment RPC failed", logging.Field("err", err.Error()))
		return nil, errx.FromRPCError(err)
	}

	return &types.CreateCommentResp{
		CommentId: result.CommentId,
	}, nil
}
