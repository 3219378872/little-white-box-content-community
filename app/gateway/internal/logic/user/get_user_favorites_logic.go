// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package user

import (
	"context"
	"gateway/internal/logic/pageutil"

	"errx"
	"esx/app/content/rpc/contentservice"
	"esx/app/content/visibility"
	"esx/app/interaction/rpc/interactionservice"
	"esx/pkg/visibilityx"
	"gateway/internal/logic/viewerstate"
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
	page := pageutil.ClampPage(req.Page)
	// 互动 GetFavoriteList 为 clamp 语义：pageSize 非正数→20、>100→20。
	pageSize := pageutil.ClampPageSizeTo(req.PageSize, 20, 100)
	// 未登录时 requesterID 为 0，由权限判断视为非 owner
	requesterID, _ := jwtx.GetUserIdFromContext(l.ctx)

	userResp, err := l.svcCtx.UserService.GetUser(l.ctx, &pb.GetUserReq{UserId: req.UserId})
	if err != nil {
		l.Errorw("UserService.GetUser RPC failed",
			logx.Field("userId", req.UserId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
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
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		l.Errorw("InteractionService.GetFavoriteList RPC failed",
			logx.Field("userId", req.UserId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}

	if len(favoriteResp.PostIds) == 0 {
		return &types.GetPostListResp{
			List:     []types.PostItem{},
			Total:    favoriteResp.Total,
			Page:     page,
			PageSize: pageSize,
		}, nil
	}

	published, err := visibility.PublishedByIDs(l.ctx, l.svcCtx.ContentService, favoriteResp.PostIds)
	if err != nil {
		l.Errorw("ContentService.GetPostsByIds RPC failed",
			logx.Field("postIds", favoriteResp.PostIds),
			logx.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}

	visible := make([]*contentservice.PostInfo, 0, len(favoriteResp.PostIds))
	postIDs := make([]int64, 0, len(favoriteResp.PostIds))
	for _, postID := range favoriteResp.PostIds {
		post := published[postID]
		if post == nil {
			continue
		}
		visible = append(visible, post)
		postIDs = append(postIDs, post.Id)
	}
	viewerID, _ := jwtx.GetOptionalUserIdFromContext(l.ctx)
	liked, favorited, err := viewerstate.Enrich(l.ctx, l.svcCtx, viewerID, postIDs)
	if err != nil {
		l.Errorw("viewerstate.Enrich failed", logx.Field("err", err.Error()))
		return nil, err
	}

	list := make([]types.PostItem, 0, len(visible))
	for _, post := range visible {
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

	total := visibilityx.AdjustPageTotal(favoriteResp.Total, len(favoriteResp.PostIds), len(visible))

	return &types.GetPostListResp{
		List:     list,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
