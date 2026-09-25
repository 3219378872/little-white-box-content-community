// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"esx/app/gateway/internal/httpxconfig"
	"esx/app/gateway/internal/logic/assistant"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
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

		client := make(chan *types.AssistantRunEvent, 16)
		completed := make(chan error, 1)
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		started := false
		start := func() {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			started = true
		}
		req.AfterSeq = resumeAfterSeq(req.AfterSeq, r.Header.Get("Last-Event-ID"))
		l := assistant.NewAssistantRunEventsLogic(ctx, svcCtx)
		threading.GoSafeCtx(ctx, func() {
			var result error = errx.NewWithCode(errx.SystemError)
			defer func() {
				completed <- result
				close(client)
			}()
			result = l.AssistantRunEvents(&req, client)
			if result != nil {
				logc.Errorw(r.Context(), "AssistantRunEventsHandler", logc.Field("error", result))
				return
			}
		})

		heartbeat := time.NewTicker(assistantSSEHeartbeatInterval)
		defer heartbeat.Stop()
		for {
			select {
			case data, ok := <-client:
				if !ok {
					select {
					case err := <-completed:
						if err != nil && ctx.Err() == nil {
							if !started {
								httpx.ErrorCtx(ctx, w, err)
							} else if writeErr := writeAssistantSSETransportError(w, err); writeErr != nil {
								logc.Errorw(ctx, "write SSE transport error", logc.Field("error", writeErr))
							}
						} else if !started {
							start()
						}
					case <-ctx.Done():
					}
					return
				}
				output, err := json.Marshal(data)
				if err != nil {
					logc.Errorw(r.Context(), "AssistantRunEventsHandler", logc.Field("error", err))
					continue
				}

				if !started {
					start()
				}
				if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", data.Seq, string(output)); err != nil {
					logc.Errorw(r.Context(), "AssistantRunEventsHandler", logc.Field("error", err))
					return
				}
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			case <-heartbeat.C:
				if !started {
					start()
				}
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

// Transport failures are not persisted run events and must not advance the resume cursor.
func writeAssistantSSETransportError(w http.ResponseWriter, err error) error {
	statusCode, body := httpxconfig.MapError(err)
	payload, marshalErr := json.Marshal(struct {
		Type      string `json:"type"`
		Error     any    `json:"error"`
		Retryable bool   `json:"retryable"`
	}{Type: "transport_error", Error: body, Retryable: statusCode >= 500 || statusCode == http.StatusTooManyRequests})
	if marshalErr != nil {
		return marshalErr
	}
	if _, writeErr := fmt.Fprintf(w, "event: transport_error\ndata: %s\n\n", payload); writeErr != nil {
		return writeErr
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}
