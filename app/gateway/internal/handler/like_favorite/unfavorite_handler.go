// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package like_favorite

import (
	"net/http"

	"gateway/internal/logic/like_favorite"
	"gateway/internal/svc"
	"gateway/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// 取消收藏
func UnfavoriteHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.UnfavoriteReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := like_favorite.NewUnfavoriteLogic(r.Context(), svcCtx)
		resp, err := l.Unfavorite(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
