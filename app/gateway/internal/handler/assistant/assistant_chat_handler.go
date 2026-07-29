// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package assistant

import (
	"encoding/json"
	"fmt"
	"net/http"

	"gateway/internal/logic/assistant"
	"gateway/internal/svc"
	"gateway/internal/types"
	"github.com/zeromicro/go-zero/core/logc"
	"github.com/zeromicro/go-zero/core/threading"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// Assistant SSE 对话
func AssistantChatHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AssistantChatReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		// Buffer size of 16 is chosen as a reasonable default to balance throughput and memory usage.
		// You can change this based on your application's needs.
		// if your go-zero version less than 1.8.1, you need to add 3 lines below.
		// w.Header().Set("Content-Type", "text/event-stream")
		// w.Header().Set("Cache-Control", "no-cache")
		// w.Header().Set("Connection", "keep-alive")
		client := make(chan *types.AssistantChatEvent, 16)

		l := assistant.NewAssistantChatLogic(r.Context(), svcCtx)
		threading.GoSafeCtx(r.Context(), func() {
			defer close(client)
			err := l.AssistantChat(&req, client)
			if err != nil {
				logc.Errorw(r.Context(), "AssistantChatHandler", logc.Field("error", err))
				return
			}
		})

		for {
			select {
			case data, ok := <-client:
				if !ok {
					return
				}
				output, err := json.Marshal(data)
				if err != nil {
					logc.Errorw(r.Context(), "AssistantChatHandler", logc.Field("error", err))
					continue
				}

				if _, err := fmt.Fprintf(w, "data: %s\n\n", string(output)); err != nil {
					logc.Errorw(r.Context(), "AssistantChatHandler", logc.Field("error", err))
					return
				}
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			case <-r.Context().Done():
				return
			}
		}
	}
}
