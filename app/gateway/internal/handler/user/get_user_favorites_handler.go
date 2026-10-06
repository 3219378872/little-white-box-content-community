package user

import (
	"context"
	"esx/app/gateway/internal/logic/user"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"

	"github.com/cloudwego/hertz/pkg/app"

	"esx/pkg/httpx"
)

// 获取用户的收藏帖子列表
func GetUserFavoritesHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req types.GetUserFavoritesReq
		if err := httpx.Parse(c, &req); err != nil {
			httpx.ErrorCtx(ctx, c, err)
			return
		}

		l := user.NewGetUserFavoritesLogic(ctx, svcCtx)
		resp, err := l.GetUserFavorites(&req)
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
		} else {
			httpx.OkJsonCtx(ctx, c, resp)
		}
	}
}
