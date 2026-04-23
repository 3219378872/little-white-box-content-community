// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package user

import (
	"gateway/internal/logic/user"
	"gateway/internal/svc"
	"gateway/internal/types"
	"net/http"

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

		l := user.NewGetUserFavoritesLogic(r.Context(), svcCtx)
		resp, err := l.GetUserFavorites(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
