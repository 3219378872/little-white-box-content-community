// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package assistant

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"esx/app/gateway/internal/logic/assistant"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"github.com/zeromicro/go-zero/core/logc"
	"github.com/zeromicro/go-zero/core/threading"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// Assistant run SSE 事件
func AssistantRunEventsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AssistantRunEventsReq
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
		client := make(chan *types.AssistantRunEvent, 16)

		ctx := r.Context()
		if req.AfterSeq == 0 {
			if header := strings.TrimSpace(r.Header.Get("Last-Event-ID")); header != "" {
				if seq, convErr := strconv.ParseInt(header, 10, 64); convErr == nil {
					req.AfterSeq = seq
				}
			}
		}
		l := assistant.NewAssistantRunEventsLogic(ctx, svcCtx)
		threading.GoSafeCtx(ctx, func() {
			defer close(client)
			err := l.AssistantRunEvents(&req, client)
			if err != nil {
				logc.Errorw(r.Context(), "AssistantRunEventsHandler", logc.Field("error", err))
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
					logc.Errorw(r.Context(), "AssistantRunEventsHandler", logc.Field("error", err))
					continue
				}

				if _, err := fmt.Fprintf(w, "data: %s\n\n", string(output)); err != nil {
					logc.Errorw(r.Context(), "AssistantRunEventsHandler", logc.Field("error", err))
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
