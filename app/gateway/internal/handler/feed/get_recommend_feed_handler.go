// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package feed

import (
	"net/http"

	"gateway/internal/logic/feed"
	"gateway/internal/svc"
	"gateway/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// 获取推荐流
func GetRecommendFeedHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GetRecommendFeedReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := feed.NewGetRecommendFeedLogic(r.Context(), svcCtx)
		resp, err := l.GetRecommendFeed(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
