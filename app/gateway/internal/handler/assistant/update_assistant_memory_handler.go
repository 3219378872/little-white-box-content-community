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

// 修改一条 Agent 记忆
func UpdateAssistantMemoryHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.UpdateAssistantMemoryReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := assistant.NewUpdateAssistantMemoryLogic(r.Context(), svcCtx)
		resp, err := l.UpdateAssistantMemory(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
