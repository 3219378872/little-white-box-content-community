// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package assistant

import (
	"net/http"

	"esx/app/gateway/internal/logic/assistant"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// 列出 Watch 命中
func ListAssistantWatchHitsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ListAssistantWatchHitsReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := assistant.NewListAssistantWatchHitsLogic(r.Context(), svcCtx)
		resp, err := l.ListAssistantWatchHits(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
