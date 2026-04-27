// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package user

import (
	"context"

	"errx"
	"esx/app/content/rpc/contentservice"

	"gateway/internal/svc"
	"gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserPostsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取用户发布的帖子列表（公开接口）
func NewGetUserPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserPostsLogic {
	return &GetUserPostsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetUserPostsLogic) GetUserPosts(req *types.GetUserPostsReq) (*types.GetPostListResp, error) {
	result, err := l.svcCtx.ContentService.GetUserPosts(l.ctx, &contentservice.GetUserPostsReq{
		UserId:   req.UserId,
		Page:     req.Page,
		PageSize: req.PageSize,
		SortBy:   req.SortBy,
	})
	if err != nil {
		l.Errorw("ContentService.GetUserPosts RPC failed",
			logx.Field("userId", req.UserId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	list := make([]types.PostItem, 0, len(result.Posts))
	for _, post := range result.Posts {
		list = append(list, types.PostItem{
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
			IsLiked:       false, // TODO: Interaction 服务实现后填充
			IsFavorited:   false, // TODO: Interaction 服务实现后填充
			CreatedAt:     post.CreatedAt,
		})
	}

	return &types.GetPostListResp{
		List:     list,
		Total:    result.Total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}
