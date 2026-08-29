// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package assistant

import (
	"net/http"

	"esx/app/gateway/internal/logic/assistant"
	"esx/app/gateway/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// 标记 Assistant 线程已读
func MarkAssistantThreadReadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := assistant.NewMarkAssistantThreadReadLogic(r.Context(), svcCtx)
		resp, err := l.MarkAssistantThreadRead()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
