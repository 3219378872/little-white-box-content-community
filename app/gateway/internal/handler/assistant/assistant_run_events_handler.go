// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package assistant

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"esx/app/gateway/internal/logic/assistant"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"github.com/zeromicro/go-zero/core/logc"
	"github.com/zeromicro/go-zero/core/threading"
	"github.com/zeromicro/go-zero/rest/httpx"
)

const assistantSSEHeartbeatInterval = 25 * time.Second

// Assistant run SSE 事件
func AssistantRunEventsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AssistantRunEventsReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		client := make(chan *types.AssistantRunEvent, 16)

		ctx := r.Context()
		req.AfterSeq = resumeAfterSeq(req.AfterSeq, r.Header.Get("Last-Event-ID"))
		l := assistant.NewAssistantRunEventsLogic(ctx, svcCtx)
		threading.GoSafeCtx(ctx, func() {
			defer close(client)
			err := l.AssistantRunEvents(&req, client)
			if err != nil {
				logc.Errorw(r.Context(), "AssistantRunEventsHandler", logc.Field("error", err))
				return
			}
		})

		heartbeat := time.NewTicker(assistantSSEHeartbeatInterval)
		defer heartbeat.Stop()
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

				if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", data.Seq, string(output)); err != nil {
					logc.Errorw(r.Context(), "AssistantRunEventsHandler", logc.Field("error", err))
					return
				}
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			case <-heartbeat.C:
				if err := writeAssistantSSEHeartbeat(w); err != nil {
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

func writeAssistantSSEHeartbeat(w http.ResponseWriter) error {
	_, err := fmt.Fprint(w, ": heartbeat\n\n")
	return err
}

func resumeAfterSeq(query int64, lastEventID string) int64 {
	header := int64(0)
	if raw := strings.TrimSpace(lastEventID); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			header = parsed
		}
	}
	if header > query {
		return header
	}
	return query
}
