// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package message

import (
	"net/http"

	"gateway/internal/logic/message"
	"gateway/internal/svc"
	"gateway/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// 发送私信
func SendMessageHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.SendMessageReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := message.NewSendMessageLogic(r.Context(), svcCtx)
		resp, err := l.SendMessage(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
