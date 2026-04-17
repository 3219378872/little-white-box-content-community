// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package user

import (
	"context"
	"encoding/json"
	"gateway/internal/logic/user"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"
	"net/http"
	"strconv"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// 获取用户的收藏帖子列表
func GetUserFavoritesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GetUserFavoritesReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		ctx := r.Context()
		ctx = tryInjectUserId(ctx, r, svcCtx)

		l := user.NewGetUserFavoritesLogic(ctx, svcCtx)
		resp, err := l.GetUserFavorites(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// tryInjectUserId 尝试从 JWT 解析 userId 注入 context。
// 无 token 或解析失败时静默返回原 context（可选认证）。
func tryInjectUserId(ctx context.Context, r *http.Request, svcCtx *svc.ServiceContext) context.Context {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ctx
	}
	cfg := jwtx.JwtConfig{
		AccessSecret: svcCtx.Config.Auth.AccessSecret,
		AccessExpire: svcCtx.Config.Auth.AccessExpire,
	}
	claims, err := jwtx.ParseToken(auth, cfg)
	if err != nil || claims == nil {
		return ctx
	}
	return context.WithValue(ctx, "userId", json.Number(strconv.FormatInt(claims.UserId, 10)))
}
