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

// 提交 Agent 推荐反馈
func SubmitAssistantRecommendFeedbackHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AssistantRecommendFeedbackReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := assistant.NewSubmitAssistantRecommendFeedbackLogic(r.Context(), svcCtx)
		resp, err := l.SubmitAssistantRecommendFeedback(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
