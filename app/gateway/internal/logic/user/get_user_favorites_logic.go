// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package user

import (
	"context"

	"errx"
	"esx/app/content/rpc/contentservice"
	"esx/app/interaction/rpc/interactionservice"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"
	"user/pb/xiaobaihe/user/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserFavoritesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取用户的收藏帖子列表
func NewGetUserFavoritesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserFavoritesLogic {
	return &GetUserFavoritesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetUserFavoritesLogic) GetUserFavorites(req *types.GetUserFavoritesReq) (*types.GetPostListResp, error) {
	// 未登录时 requesterID 为 0，由权限判断视为非 owner
	requesterID, _ := jwtx.GetUserIdFromContext(l.ctx)

	userResp, err := l.svcCtx.UserService.GetUser(l.ctx, &pb.GetUserReq{UserId: req.UserId})
	if err != nil {
		l.Errorw("UserService.GetUser RPC failed",
			logx.Field("userId", req.UserId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if userResp.User == nil {
		return nil, errx.NewWithCode(errx.UserNotFound)
	}

	isOwner := requesterID != 0 && requesterID == req.UserId
	// DB 约定：1=公开，2=仅自己可见
	isPublic := userResp.User.FavoritesVisibility == 1
	if !isOwner && !isPublic {
		return nil, errx.NewWithCode(errx.FavoritesPrivate)
	}

	favoriteResp, err := l.svcCtx.InteractionService.GetFavoriteList(l.ctx, &interactionservice.GetFavoriteListReq{
		UserId:   req.UserId,
		Page:     int32(req.Page),
		PageSize: int32(req.PageSize),
	})
	if err != nil {
		l.Errorw("InteractionService.GetFavoriteList RPC failed",
			logx.Field("userId", req.UserId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if len(favoriteResp.PostIds) == 0 {
		return &types.GetPostListResp{
			List:     []types.PostItem{},
			Total:    favoriteResp.Total,
			Page:     req.Page,
			PageSize: req.PageSize,
		}, nil
	}

	postsResp, err := l.svcCtx.ContentService.GetPostsByIds(l.ctx, &contentservice.GetPostsByIdsReq{
		PostIds: favoriteResp.PostIds,
	})
	if err != nil {
		l.Errorw("ContentService.GetPostsByIds RPC failed",
			logx.Field("postIds", favoriteResp.PostIds),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	list := make([]types.PostItem, 0, len(postsResp.Posts))
	for _, post := range postsResp.Posts {
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
			CreatedAt:     post.CreatedAt,
		})
	}

	return &types.GetPostListResp{
		List:     list,
		Total:    favoriteResp.Total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}
