// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package comment

import (
	"context"
	"esx/pkg/pageutil"

	"esx/app/content/rpc/contentservice"
	"esx/pkg/errx"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetCommentRepliesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取评论的回复列表（楼中楼分页，时间正序）
func NewGetCommentRepliesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCommentRepliesLogic {
	return &GetCommentRepliesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetCommentRepliesLogic) GetCommentReplies(req *types.GetCommentRepliesReq) (resp *types.GetCommentRepliesResp, err error) {
	// 与内容 RPC 的 clamp 语义保持一致：回传实际使用的 pageSize。
	pageSize := pageutil.ClampPageSize(req.PageSize)
	page := pageutil.ClampPage(req.Page)
	result, err := l.svcCtx.ContentService.GetCommentReplies(l.ctx, &contentservice.GetCommentRepliesReq{
		CommentId: req.CommentId,
		Page:      page,
		PageSize:  pageSize,
	})
	if err != nil {
		l.Errorw("ContentService.GetCommentReplies RPC failed",
			logx.Field("commentId", req.CommentId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}

	list := make([]types.CommentItem, 0, len(result.Comments))
	for _, c := range result.Comments {
		list = append(list, toCommentItem(c))
	}

	return &types.GetCommentRepliesResp{
		List:     list,
		Total:    result.Total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
