// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package user

import (
	"context"

	"errx"
	"esx/app/content/rpc/contentservice"
	"gateway/internal/logic/viewerstate"

	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"

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
		return nil, errx.FromRPCError(err)
	}

	postIDs := make([]int64, 0, len(result.Posts))
	for _, post := range result.Posts {
		if post != nil && post.Id > 0 {
			postIDs = append(postIDs, post.Id)
		}
	}
	viewerID, _ := jwtx.GetOptionalUserIdFromContext(l.ctx)
	liked, favorited, err := viewerstate.Enrich(l.ctx, l.svcCtx, viewerID, postIDs)
	if err != nil {
		l.Errorw("viewerstate.Enrich failed", logx.Field("err", err.Error()))
		return nil, err
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
			IsLiked:       liked[post.Id],
			IsFavorited:   favorited[post.Id],
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
