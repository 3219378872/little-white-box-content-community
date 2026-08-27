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

// 标记 Watch 命中已读
func MarkAssistantWatchHitsReadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.MarkAssistantWatchHitsReadReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := assistant.NewMarkAssistantWatchHitsReadLogic(r.Context(), svcCtx)
		resp, err := l.MarkAssistantWatchHitsRead(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
