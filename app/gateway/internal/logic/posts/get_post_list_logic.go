// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package posts

import (
	"context"

	"errx"
	"esx/app/content/contentservice"
	"jwtx"

	"gateway/internal/svc"
	"gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPostListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取帖子列表
func NewGetPostListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostListLogic {
	return &GetPostListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetPostListLogic) GetPostList(req *types.GetPostListReq) (resp *types.GetPostListResp, err error) {
	rpcReq := &contentservice.GetPostListReq{
		Page:     req.Page,
		PageSize: req.PageSize,
		SortBy:   req.SortBy,
	}
	if userId, ok := jwtx.GetOptionalUserIdFromContext(l.ctx); ok {
		rpcReq.UserId = userId
	}

	result, err := l.svcCtx.ContentService.GetPostList(l.ctx, rpcReq)
	if err != nil {
		l.Errorw("ContentService.GetPostList RPC failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	list := make([]types.PostItem, 0, len(result.Posts))
	for _, post := range result.Posts {
		list = append(list, types.PostItem{
			Id:           post.Id,
			AuthorId:     post.AuthorId,
			Title:        post.Title,
			Content:      post.Content,
			Images:       post.Images,
			Tags:         post.Tags,
			ViewCount:    post.ViewCount,
			LikeCount:    post.LikeCount,
			CommentCount: post.CommentCount,
			CreatedAt:    post.CreatedAt,
		})
	}

	return &types.GetPostListResp{
		List:     list,
		Total:    result.Total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}
