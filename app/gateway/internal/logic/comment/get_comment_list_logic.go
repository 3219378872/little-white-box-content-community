// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package comment

import (
	"context"
	"gateway/internal/logic/pageutil"

	"errx"
	"esx/app/content/rpc/contentservice"

	"gateway/internal/svc"
	"gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetCommentListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取评论列表
func NewGetCommentListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCommentListLogic {
	return &GetCommentListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetCommentListLogic) GetCommentList(req *types.GetCommentListReq) (resp *types.GetCommentListResp, err error) {
	// 与内容 RPC 的 clamp 语义保持一致：回传实际使用的 pageSize。
	pageSize := pageutil.ClampPageSize(req.PageSize)
	page := pageutil.ClampPage(req.Page)
	result, err := l.svcCtx.ContentService.GetCommentList(l.ctx, &contentservice.GetCommentListReq{
		PostId:   req.PostId,
		Page:     page,
		PageSize: pageSize,
		SortBy:   req.SortBy,
	})
	if err != nil {
		l.Errorw("ContentService.GetCommentList RPC failed",
			logx.Field("postId", req.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}

	list := make([]types.CommentItem, 0, len(result.Comments))
	for _, c := range result.Comments {
		list = append(list, types.CommentItem{
			Id:          c.Id,
			UserId:      c.UserId,
			ParentId:    c.ParentId,
			ReplyUserId: c.ReplyUserId,
			Content:     c.Content,
			LikeCount:   c.LikeCount,
			CreatedAt:   c.CreatedAt,
		})
	}

	return &types.GetCommentListResp{
		List:     list,
		Total:    result.Total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
