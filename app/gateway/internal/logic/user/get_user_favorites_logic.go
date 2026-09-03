// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package user

import (
	"context"

	"esx/app/content/rpc/contentservice"
	"esx/app/content/visibility"
	"esx/app/gateway/internal/logic/authorx"
	"esx/app/gateway/internal/logic/postmap"
	"esx/app/gateway/internal/logic/rpcx"
	"esx/app/gateway/internal/logic/viewerstate"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/cursorx"
	"esx/pkg/errx"
	"esx/pkg/jwtx"
	"esx/pkg/pageutil"

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
	page := int32(1)
	if req.Cursor != "" {
		data, err := cursorx.Decode(req.Cursor)
		if err != nil {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		p := data["p"]
		if p <= 0 || p > 10_000 {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		page = int32(p)
	}
	pageSize := pageutil.ClampPageSizeTo(req.PageSize, pageutil.DefaultPageSize, pageutil.InteractionMaxPageSize)
	requesterID, _ := jwtx.GetOptionalUserIdFromContext(l.ctx)

	userResp, err := l.svcCtx.UserService.GetUser(l.ctx, &pb.GetUserReq{UserId: req.UserId})
	if err != nil {
		return nil, rpcx.Error(l.Logger, "UserService.GetUser", err, logx.Field("userId", req.UserId))
	}
	if userResp.User == nil {
		return nil, errx.NewWithCode(errx.UserNotFound)
	}

	isOwner := requesterID != 0 && requesterID == req.UserId
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
		return nil, rpcx.Error(l.Logger, "InteractionService.GetFavoriteList", err, logx.Field("userId", req.UserId))
	}

	if len(favoriteResp.PostIds) == 0 {
		return &types.GetPostListResp{List: []types.PostItem{}}, nil
	}

	published, err := visibility.PublishedByIDs(l.ctx, l.svcCtx.ContentService, favoriteResp.PostIds)
	if err != nil {
		return nil, rpcx.Error(l.Logger, "ContentService.GetPostsByIds", err, logx.Field("postIds", favoriteResp.PostIds))
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

	var nextCursor string
	if len(favoriteResp.PostIds) >= int(pageSize) {
		token, err := cursorx.Encode(cursorx.Data{"p": int64(page) + 1})
		if err != nil {
			l.Errorw("encode favorites cursor failed", logx.Field("err", err.Error()))
		} else {
			nextCursor = token
		}
	}

	return &types.GetPostListResp{
		List:       postmap.Items(visible, liked, favorited, authorx.LoadSoft(l.ctx, l.svcCtx, authorx.PostAuthorIDs(visible))),
		NextCursor: nextCursor,
	}, nil
}
