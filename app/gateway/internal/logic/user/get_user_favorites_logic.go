// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package user

import (
	"context"
	"errx"
	"fmt"
	"jwtx"
	"user/pb/xiaobaihe/user/pb"

	"gateway/internal/svc"
	"gateway/internal/types"

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
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}
	if userResp.User == nil {
		return nil, fmt.Errorf("用户不存在: userId=%d", req.UserId)
	}

	isOwner := requesterID != 0 && requesterID == req.UserId
	// DB 约定：1=公开，2=仅自己可见
	isPublic := userResp.User.FavoritesVisibility == 1
	if !isOwner && !isPublic {
		return nil, errx.NewWithCode(errx.FavoritesPrivate)
	}

	// TODO: Interaction 服务实现后接入批量查询，当前返回空列表
	return &types.GetPostListResp{
		List:     []types.PostItem{},
		Total:    0,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}
