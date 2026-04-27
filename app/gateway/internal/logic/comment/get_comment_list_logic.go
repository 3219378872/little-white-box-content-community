// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package comment

import (
	"context"

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
	result, err := l.svcCtx.ContentService.GetCommentList(l.ctx, &contentservice.GetCommentListReq{
		PostId:   req.PostId,
		Page:     req.Page,
		PageSize: req.PageSize,
		SortBy:   req.SortBy,
	})
	if err != nil {
		l.Errorw("ContentService.GetCommentList RPC failed",
			logx.Field("postId", req.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
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
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}
