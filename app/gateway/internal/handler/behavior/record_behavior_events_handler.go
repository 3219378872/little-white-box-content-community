// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package behavior

import (
	"net/http"

	"gateway/internal/logic/behavior"
	"gateway/internal/svc"
	"gateway/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// 批量记录客户端行为
func RecordBehaviorEventsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RecordBehaviorEventsReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := behavior.NewRecordBehaviorEventsLogic(r.Context(), svcCtx)
		resp, err := l.RecordBehaviorEvents(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
