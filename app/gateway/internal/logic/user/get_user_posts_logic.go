// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package user

import (
	"context"
	"esx/app/gateway/internal/logic/pageutil"

	"esx/app/content/rpc/contentservice"
	"esx/app/gateway/internal/logic/viewerstate"
	"esx/pkg/errx"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/jwtx"

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
	// 与内容 RPC 的 clamp 语义保持一致：回传实际使用的 pageSize。
	pageSize := pageutil.ClampPageSize(req.PageSize)
	page := pageutil.ClampPage(req.Page)
	result, err := l.svcCtx.ContentService.GetUserPosts(l.ctx, &contentservice.GetUserPostsReq{
		UserId:   req.UserId,
		Page:     page,
		PageSize: pageSize,
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
			Status:        post.Status,
			ViewCount:     post.ViewCount,
			LikeCount:     post.LikeCount,
			CommentCount:  post.CommentCount,
			FavoriteCount: post.FavoriteCount,
			IsLiked:       liked[post.Id],
			IsFavorited:   favorited[post.Id],
			Revision:      post.Revision,
			CreatedAt:     post.CreatedAt,
		})
	}

	return &types.GetPostListResp{
		List:     list,
		Total:    result.Total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
