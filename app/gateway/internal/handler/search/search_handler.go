// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package search

import (
	"net/http"

	"gateway/internal/logic/search"
	"gateway/internal/svc"
	"gateway/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// 综合搜索
func SearchHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.SearchReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := search.NewSearchLogic(r.Context(), svcCtx)
		resp, err := l.Search(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
