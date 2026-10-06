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

// DeleteCommentLogic 承载 DeleteComment 接口的业务逻辑；每个请求新建一个实例。
type DeleteCommentLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 删除评论
func NewDeleteCommentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteCommentLogic {
	return &DeleteCommentLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// DeleteComment 删除评论；是否有权删除由内容服务判断。
func (l *DeleteCommentLogic) DeleteComment(req *types.DeleteCommentReq) (resp *types.DeleteCommentResp, err error) {
	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	_, err = l.svcCtx.ContentService.DeleteComment(l.ctx, &contentservice.DeleteCommentReq{
		CommentId: req.CommentId,
		UserId:    userId,
	})
	if err != nil {
		l.Errorw("ContentService.DeleteComment RPC failed",
			logging.Field("commentId", req.CommentId),
			logging.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}

	return &types.DeleteCommentResp{}, nil
}
