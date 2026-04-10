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

type GetPostLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取帖子详情
func NewGetPostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostLogic {
	return &GetPostLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetPostLogic) GetPost(req *types.GetPostReq) (resp *types.GetPostResp, err error) {
	// 未登录用户 userId 为 0，RPC 层根据 userId 判断互动状态
	userId, _ := l.ctx.Value("userId").(int64)

	result, err := l.svcCtx.ContentService.GetPost(l.ctx, &contentservice.GetPostReq{
		PostId: req.PostId,
		UserId: userId,
	})
	if err != nil {
		return nil, fmt.Errorf("获取帖子失败: %w", err)
	}

	post := result.Post
	if post == nil {
		return nil, fmt.Errorf("帖子不存在")
	}

	return &types.GetPostResp{
		Id:            post.Id,
		AuthorId:      post.AuthorId,
		Title:         post.Title,
		Content:       post.Content,
		Images:        post.Images,
		Tags:          post.Tags,
		ViewCount:     post.ViewCount,
		LikeCount:     post.LikeCount,
		CommentCount:  post.CommentCount,
		FavoriteCount: post.FavoriteCount,
		IsLiked:       result.IsLiked,
		IsFavorited:   result.IsFavorited,
		CreatedAt:     post.CreatedAt,
	}, nil
}
