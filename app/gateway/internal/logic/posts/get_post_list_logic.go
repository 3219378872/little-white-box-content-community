// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package posts

import (
	"context"
	"esx/app/content/contentservice"
	"fmt"

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
	result, err := l.svcCtx.ContentService.GetPostList(l.ctx, &contentservice.GetPostListReq{
		Page:     req.Page,
		PageSize: req.PageSize,
		SortBy:   req.SortBy,
	})
	if err != nil {
		return nil, fmt.Errorf("获取帖子列表失败: %w", err)
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
