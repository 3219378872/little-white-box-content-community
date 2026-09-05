package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
)

func (e *Engine) completeModelText(ctx context.Context, run store.Run, result llm.Result) error {
	text := result.Text
	switch run.Source {
	case store.SourceWatch:
		return e.completeWatchWithStream(ctx, run, text,
			prompt.EncodeTurn(prompt.Turn{Role: store.RoleAssistant, Content: text}), !result.Streamed, result.StreamID)
	case store.SourceMemoryReview:
		return e.completeMemoryReview(ctx, run)
	default:
		return e.finishWithMessageEvent(ctx, run, store.StatusDone, store.EventDone,
			store.EventPayload{Text: text, StreamID: result.StreamID}, text,
			prompt.EncodeTurn(prompt.Turn{Role: store.RoleAssistant, Content: text}), !result.Streamed, result.StreamID)
	}
}

func (e *Engine) completeWatchWithStream(ctx context.Context, run store.Run, text string, apiContent []byte, emitToken bool, streamID string) error {
	payload := decodeWatchRunPayload(run.QueuedPayload)
	if payload.BucketID <= 0 {
		return e.fail(ctx, run, "WATCH_BUCKET_MISSING", "watch delivery bucket is missing")
	}
	if _, err := e.currentWatchHits(ctx, run, payload); err != nil {
		if !emitToken && streamID != "" {
			if _, resetErr := e.appendEvent(ctx, run, store.EventResponseReset, store.EventPayload{StreamID: streamID}); resetErr != nil {
				return resetErr
			}
		}
		if errors.Is(err, errNoVisibleWatchHits) {
			return e.dismissWatchRun(ctx, run)
		}
		return err
	}
	now := store.NowMs()
	err := e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
		if text != "" && emitToken {
			if _, err := appendEventTx(ctx, tx, run, store.EventToken, store.EventPayload{Text: text, StreamID: streamID}, now); err != nil {
				return err
			}
		}
		msg, err := tx.InsertMessage(ctx, store.Message{
			UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, Role: store.RoleAssistant,
			Kind: store.KindWatch, Content: text, APIContent: apiContent, Visible: true, Unread: true, CreatedAtMs: now,
		})
		if err != nil {
			return err
		}
		if err := insertMessageOutbox(ctx, tx, msg); err != nil {
			return err
		}
		thread, err := tx.LockThread(ctx, run.UserID)
		if err != nil {
			return err
		}
		thread.UnreadCount++
		thread.LastMessageID = msg.ID
		thread.LastMessagePreview = store.Preview(text, 80)
		thread.LastMessageAtMs = now
		thread.UpdatedAtMs = now
		if err := tx.SaveThread(ctx, *thread); err != nil {
			return err
		}
		if err := tx.FinishWatchDelivery(ctx, payload.BucketID, run.UserID, run.ID, store.StatusDone, now); err != nil {
			return err
		}
		_, finishErr := finishRunTx(ctx, tx, run, store.StatusDone, store.EventDone, store.EventPayload{Text: text, StreamID: streamID}, now)
		return finishErr
	})
	if err != nil {
		return err
	}
	e.wake(ctx, run.ID)
	return nil
}

func (e *Engine) completeMemoryReview(ctx context.Context, run store.Run) error {
	fresh, err := e.Store.GetRun(ctx, run.ID)
	if err != nil {
		return err
	}
	if fresh != nil && fresh.Status == store.StatusDone {
		return nil
	}
	changeIDs, err := e.memoryChangeIDs(ctx, run.ID)
	if err != nil {
		return err
	}
	now := store.NowMs()
	err = e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
		existing, err := tx.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
		if err != nil {
			return err
		}
		seen := make(map[int64]struct{})
		for _, msg := range existing {
			if msg.Kind == store.KindMemoryChanged && msg.ChangeID > 0 {
				seen[msg.ChangeID] = struct{}{}
			}
		}
		thread, err := tx.LockThread(ctx, run.UserID)
		if err != nil {
			return err
		}
		for _, changeID := range changeIDs {
			if _, ok := seen[changeID]; ok {
				continue
			}
			msg, err := tx.InsertMessage(ctx, store.Message{
				UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, Role: store.RoleSystem,
				Kind: store.KindMemoryChanged, Content: "记忆已更新，可撤销。", Visible: true,
				Unread: false, ChangeID: changeID, CreatedAtMs: now,
			})
			if err != nil {
				return err
			}
			thread.LastMessageID = msg.ID
			thread.LastMessagePreview = msg.Content
			thread.LastMessageAtMs = now
			thread.UpdatedAtMs = now
			if _, err := appendEventTx(ctx, tx, run, store.EventMemoryChanged, store.EventPayload{ChangeID: changeID}, now); err != nil {
				return err
			}
		}
		if err := tx.SaveThread(ctx, *thread); err != nil {
			return err
		}
		_, err = finishRunTx(ctx, tx, run, store.StatusDone, store.EventDone, store.EventPayload{}, now)
		return err
	})
	if err == nil {
		e.wake(ctx, run.ID)
	}
	return err
}

func (e *Engine) memoryChangeIDs(ctx context.Context, runID int64) ([]int64, error) {
	calls, err := e.Store.ListToolCalls(ctx, runID)
	if err != nil {
		return nil, err
	}
	seen := make(map[int64]struct{})
	for _, call := range calls {
		if call.Status != "success" {
			continue
		}
		switch call.Tool {
		case tool.AddMemory, tool.ReplaceMemory, tool.RemoveMemory, tool.BatchMemory:
		default:
			continue
		}
		var result struct {
			ChangeIDs []int64 `json:"change_ids"`
		}
		if json.Unmarshal([]byte(call.ResultJSON), &result) != nil {
			continue
		}
		for _, id := range result.ChangeIDs {
			if id > 0 {
				seen[id] = struct{}{}
			}
		}
	}
	out := make([]int64, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func insertMessageOutbox(ctx context.Context, tx store.Store, msg store.Message) error {
	payload, _ := json.Marshal(map[string]any{
		"userId": msg.UserID, "sessionId": msg.SessionID, "messageId": msg.ID,
		"role": msg.Role, "content": msg.Content, "createdAtMs": msg.CreatedAtMs,
		"deleted": false, "compacted": msg.Compacted,
	})
	return tx.InsertOutbox(ctx, store.Outbox{
		UserID: msg.UserID, MessageID: msg.ID, Op: store.IndexOpUpsert,
		PayloadJSON: string(payload), CreatedAtMs: msg.CreatedAtMs,
	})
}

func finishRunTx(ctx context.Context, tx store.Store, run store.Run, status, eventType string, payload store.EventPayload, now int64) (store.Event, error) {
	run.Status = status
	run.Phase = store.PhaseDone
	run.EndedAtMs = now
	run.LastActivityAtMs = now
	if status == store.StatusCancelled {
		run.CancelRequested = true
	}
	if payload.ErrorCode != "" {
		run.ErrorCode = payload.ErrorCode
	}
	if err := tx.UpdateRun(ctx, run); err != nil {
		return store.Event{}, err
	}
	return appendEventTx(ctx, tx, run, eventType, payload, now)
}

func appendEventTx(ctx context.Context, tx store.Store, run store.Run, eventType string, payload store.EventPayload, now int64) (store.Event, error) {
	payload.SessionID = run.SessionID
	raw, _ := json.Marshal(payload)
	return tx.InsertEvent(ctx, run.ID, eventType, raw, now)
}

func (e *Engine) wake(ctx context.Context, runID int64) {
	if e.Notify != nil {
		_ = e.Notify.Wake(ctx, runID)
	}
}
