// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package assistant

import (
	"net/http"

	"esx/app/gateway/internal/logic/assistant"
	"esx/app/gateway/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// 查询 Agent 能力授权状态（AGNT-004）
func GetAgentConsentHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := assistant.NewGetAgentConsentLogic(r.Context(), svcCtx)
		resp, err := l.GetAgentConsent()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
