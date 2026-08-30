package runtime

import (
	"context"
	"encoding/json"
	"time"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/pkg/errx"
)

var publicEventTypes = map[string]struct{}{
	store.EventRunStarted: {}, store.EventToken: {}, store.EventResponseReset: {}, store.EventToolCall: {},
	store.EventToolResult: {}, store.EventConfirmRequired: {}, store.EventSourceCard: {},
	store.EventMemoryChanged: {}, store.EventDone: {}, store.EventError: {},
}

func AppendEvent(ctx context.Context, st store.Store, notify store.Notifier, run store.Run, eventType string, payload store.EventPayload) (store.Event, error) {
	if _, ok := publicEventTypes[eventType]; !ok {
		return store.Event{}, nil
	}
	payload.SessionID = run.SessionID
	raw, _ := json.Marshal(payload)
	ev, err := st.InsertEvent(ctx, run.ID, eventType, raw, store.NowMs())
	if err != nil {
		return store.Event{}, err
	}
	if notify != nil {
		_ = notify.Wake(ctx, run.ID)
	}
	return ev, nil
}

func ToPB(ev store.Event) *pb.RunEvent {
	var payload store.EventPayload
	_ = json.Unmarshal(ev.PayloadJSON, &payload)
	out := &pb.RunEvent{
		RunId: ev.RunID, Seq: ev.Seq, Type: ev.Type, Text: payload.Text, Degraded: payload.Degraded,
		ErrorCode: payload.ErrorCode, SessionId: payload.SessionID, ChangeId: payload.ChangeID, StreamId: payload.StreamID,
	}
	if payload.ToolCall != nil {
		out.ToolCall = &pb.ToolCallInfo{
			CallId: payload.ToolCall.CallID, Tool: payload.ToolCall.Tool,
			Summary: payload.ToolCall.Summary, PayloadJson: payload.ToolCall.PayloadJSON,
		}
	}
	if payload.SourceCard != nil {
		out.SourceCard = &pb.SourceCard{
			Handle: payload.SourceCard.Handle, Kind: payload.SourceCard.Kind, AuthorityId: payload.SourceCard.AuthorityID,
			Title: payload.SourceCard.Title, Revision: payload.SourceCard.Revision, PayloadJson: payload.SourceCard.PayloadJSON,
		}
	}
	return out
}

func Subscribe(ctx context.Context, st store.Store, notify store.Notifier, userID, runID, afterSeq int64, emit func(*pb.RunEvent) error) error {
	run, err := st.GetRun(ctx, runID)
	if err != nil {
		return errx.NewWithCode(errx.NotFound)
	}
	if run.UserID != userID {
		return errx.NewWithCode(errx.PermissionDenied)
	}
	last := afterSeq
	send := func() error {
		events, err := st.ListEventsAfter(ctx, runID, last)
		if err != nil {
			return err
		}
		for _, ev := range events {
			if _, ok := publicEventTypes[ev.Type]; !ok {
				last = ev.Seq
				continue
			}
			if err := emit(ToPB(ev)); err != nil {
				return err
			}
			last = ev.Seq
		}
		return nil
	}
	if err := send(); err != nil {
		return err
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var lastWake string
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if notify != nil {
				token, err := notify.WakeToken(ctx, runID)
				if err == nil && token == lastWake {
					fresh, ferr := st.GetRun(ctx, runID)
					if ferr == nil && store.IsTerminalStatus(fresh.Status) {
						if err := send(); err != nil {
							return err
						}
						return nil
					}
					continue
				}
				if err == nil {
					lastWake = token
				}
			}
			if err := send(); err != nil {
				return err
			}
			fresh, ferr := st.GetRun(ctx, runID)
			if ferr == nil && store.IsTerminalStatus(fresh.Status) {
				return nil
			}
		}
	}
}

func Confirm(ctx context.Context, st store.Store, userID, runID int64, callID string, approved bool) error {
	if userID <= 0 || runID <= 0 || callID == "" {
		return errx.NewWithCode(errx.ParamError)
	}
	run, err := st.GetRun(ctx, runID)
	if err != nil {
		return errx.NewWithCode(errx.NotFound)
	}
	if run.UserID != userID {
		return errx.NewWithCode(errx.PermissionDenied)
	}
	conf, err := st.GetConfirmation(ctx, runID, callID)
	if err != nil {
		return err
	}
	if conf == nil || conf.Status != store.ConfirmPending {
		return errx.NewWithCode(errx.ParamError)
	}
	resolved, err := st.ResolveConfirmation(ctx, userID, runID, callID, conf.CanonicalArgsDigest, approved, store.NowMs())
	if err != nil {
		return err
	}
	if resolved == nil || resolved.Status == store.ConfirmPending {
		return errx.NewWithCode(errx.ParamError)
	}
	if resolved.Status != store.ConfirmApproved && resolved.Status != store.ConfirmRejected {
		return errx.NewWithCode(errx.ParamError)
	}
	if !approved && resolved.Status != store.ConfirmRejected {
		return errx.NewWithCode(errx.ParamError)
	}
	if approved && resolved.Status != store.ConfirmApproved {
		return errx.NewWithCode(errx.ParamError)
	}
	return nil
}
