package runtime

import (
	"context"
	"encoding/json"
	"sort"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
)

func (e *Engine) completeModelText(ctx context.Context, run store.Run, result llm.Result) error {
	text := result.Text
	switch run.Source {
	case store.SourceMemoryReview:
		return e.completeMemoryReview(ctx, run)
	default:
		return e.finishMessage(ctx, run, terminalOutcome{
			status: store.StatusDone, eventType: store.EventDone,
			payload: store.EventPayload{Text: text, StreamID: result.StreamID},
		}.withAssistantText(text, result))
	}
}

func (e *Engine) completeMemoryReview(ctx context.Context, run store.Run) error {
	fresh, err := e.Store.GetRun(ctx, run.ID)
	if err != nil {
		return err
	}
	if fresh != nil && fresh.Status == store.StatusDone {
		return nil
	}
	return e.finish(ctx, run, store.StatusDone, store.EventDone, store.EventPayload{})
}

func publishMemoryChanges(ctx context.Context, tx store.Store, run store.Run, thread *store.Thread, now int64) error {
	changeIDs, err := memoryChangeIDs(ctx, tx, run.ID)
	if err != nil {
		return err
	}
	if len(changeIDs) == 0 {
		return nil
	}
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
	return nil
}

func memoryChangeIDs(ctx context.Context, st store.Store, runID int64) ([]int64, error) {
	calls, err := st.ListToolCalls(ctx, runID)
	if err != nil {
		return nil, err
	}
	seen := make(map[int64]struct{})
	for _, call := range calls {
		if call.Status != "success" && call.Status != "replay" {
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
