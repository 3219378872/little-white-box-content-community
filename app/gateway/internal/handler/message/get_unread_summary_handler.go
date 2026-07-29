// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package message

import (
	"net/http"

	"gateway/internal/logic/message"
	"gateway/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// 获取未读汇总
func GetUnreadSummaryHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := message.NewGetUnreadSummaryLogic(r.Context(), svcCtx)
		resp, err := l.GetUnreadSummary()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
