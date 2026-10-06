package assistant

import (
	"context"
	"encoding/json"
	"esx/app/gateway/internal/httpxconfig"
	"esx/app/gateway/internal/logic/assistant"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/httpx"
	"esx/pkg/logging"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/sse"
)

const assistantSSEHeartbeatInterval = 25 * time.Second

func AssistantRunEventsHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req types.AssistantRunEventsReq
		if err := httpx.Parse(c, &req); err != nil {
			httpx.ErrorCtx(ctx, c, err)
			return
		}
		req.AfterSeq = resumeAfterSeq(req.AfterSeq, string(c.GetHeader("Last-Event-ID")))
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		events := make(chan *types.AssistantRunEvent, 16)
		completed := make(chan error, 1)
		logic := assistant.NewAssistantRunEventsLogic(ctx, svcCtx)
		go func() {
			// A panic in the logic still ends the stream with a SystemError.
			var result error
			result = errx.NewWithCode(errx.SystemError)
			defer func() {
				if recover() != nil {
					result = errx.NewWithCode(errx.SystemError)
				}
				completed <- result
				close(events)
			}()
			result = logic.AssistantRunEvents(&req, events)
		}()
		var writer *sse.Writer
		start := func() {
			if writer == nil {
				writer = sse.NewWriter(c)
				c.Response.Header.SetContentType("text/event-stream")
			}
		}
		defer func() {
			if writer != nil {
				_ = writer.Close()
			}
		}()
		ticker := time.NewTicker(assistantSSEHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case event, ok := <-events:
				if !ok {
					err := <-completed
					if err != nil && ctx.Err() == nil {
						if writer == nil {
							httpx.ErrorCtx(ctx, c, err)
						} else {
							_ = writeAssistantSSETransportError(writer, err)
						}
					} else if writer == nil && ctx.Err() == nil {
						start()
					}
					return
				}
				data, err := json.Marshal(event)
				if err != nil {
					logging.WithContext(ctx).Errorw("SSE event encoding failed")
					return
				}
				start()
				if err = writer.WriteEvent(strconv.FormatInt(event.Seq, 10), "", data); err != nil {
					return
				}
			case <-ticker.C:
				start()
				if err := writeAssistantSSEHeartbeat(c); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}
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

func writeAssistantSSETransportError(writer *sse.Writer, err error) error {
	code, body := httpxconfig.MapError(err)
	data, e := json.Marshal(struct {
		Type      string `json:"type"`
		Error     any    `json:"error"`
		Retryable bool   `json:"retryable"`
	}{"transport_error", body, code >= 500 || code == 429})
	if e != nil {
		return e
	}
	return writer.WriteEvent("", "transport_error", data)
}

func writeAssistantSSEHeartbeat(c *app.RequestContext) error {
	w := c.Response.GetHijackWriter()
	if _, err := w.Write([]byte(": heartbeat\n\n")); err != nil {
		return err
	}
	return w.Flush()
}
