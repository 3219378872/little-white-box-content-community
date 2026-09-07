package runtime

import (
	"context"
	"encoding/json"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"strings"
)

func (e *Engine) ensureStarted(ctx context.Context, run store.Run) error {
	seq, err := e.Store.MaxEventSeq(ctx, run.ID)
	if err != nil {
		return err
	}
	if seq == 0 {
		_, err = e.appendEvent(ctx, run, store.EventRunStarted, store.EventPayload{})
	}
	return err
}

func (e *Engine) resourceLimit(ctx context.Context, run store.Run) error {
	journals, err := e.Store.ListSuccessfulJournal(ctx, run.UserID, run.RequestID)
	if err != nil {
		return err
	}
	summary := make([]string, 0, len(journals))
	for _, row := range journals {
		summary = append(summary, row.Tool)
	}
	return e.finish(ctx, run, store.StatusError, store.EventError, store.EventPayload{
		ErrorCode: "AGENT_RESOURCE_LIMIT", Text: "资源预算已耗尽", Journal: strings.Join(summary, ","),
	})
}

func (e *Engine) fail(ctx context.Context, run store.Run, code, text string) error {
	return e.finish(ctx, run, store.StatusError, store.EventError, store.EventPayload{ErrorCode: code, Text: text})
}

func (e *Engine) cancel(ctx context.Context, run store.Run) error {
	run.CancelRequested = true
	return e.finish(ctx, run, store.StatusCancelled, store.EventError, store.EventPayload{ErrorCode: "CANCELLED", Text: "run cancelled"})
}

func (e *Engine) finish(ctx context.Context, run store.Run, status, eventType string, payload store.EventPayload) error {
	return e.finishWithMessage(ctx, run, status, eventType, payload, "", nil)
}

func (e *Engine) finishWithMessage(
	ctx context.Context,
	run store.Run,
	status, eventType string,
	payload store.EventPayload,
	message string,
	apiContent []byte,
) error {
	return e.finishWithMessageEvent(ctx, run, status, eventType, payload, message, apiContent, true, "")
}

func (e *Engine) finishWithMessageEvent(
	ctx context.Context,
	run store.Run,
	status, eventType string,
	payload store.EventPayload,
	message string,
	apiContent []byte,
	emitToken bool,
	streamID string,
) error {
	return e.finishMessage(ctx, run, status, eventType, payload, message, apiContent, emitToken, streamID, nil)
}

func (e *Engine) finishMessage(ctx context.Context, run store.Run, status, eventType string, payload store.EventPayload, message string, apiContent []byte, emitToken bool, streamID string, before func(context.Context, store.Store) error) error {
	now := store.NowMs()
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
	err := e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
		fresh, err := tx.GetRun(ctx, run.ID)
		if err != nil {
			return err
		}
		cancelWon := status != store.StatusCancelled && fresh.CancelRequested
		if cancelWon {
			status = store.StatusCancelled
			eventType = store.EventError
			payload = store.EventPayload{ErrorCode: "CANCELLED", Text: "run cancelled"}
			message = ""
			apiContent = nil
			emitToken = false
			streamID = ""
			run.Status = store.StatusCancelled
			run.CancelRequested = true
			run.ErrorCode = "CANCELLED"
		}
		if status != store.StatusDone {
			if err := closePendingCalls(ctx, tx, run, status); err != nil {
				return err
			}
		}
		if before != nil && !cancelWon {
			if err := before(ctx, tx); err != nil {
				return err
			}
		}
		if err := tx.UpdateRun(ctx, run); err != nil {
			return err
		}
		thread, err := tx.LockThread(ctx, run.UserID)
		if err != nil {
			return err
		}
		if message != "" && run.Source != store.SourceMemoryReview {
			if emitToken {
				if _, err := AppendEvent(ctx, tx, nil, run, store.EventToken, store.EventPayload{Text: message, StreamID: streamID}); err != nil {
					return err
				}
			}
			if len(apiContent) == 0 {
				apiContent = prompt.EncodeTurn(prompt.Turn{Role: store.RoleAssistant, Content: message})
			}
			if streamID != "" && payload.StreamID == "" {
				payload.StreamID = streamID
			}
			kind := store.KindMessage
			if run.Source == store.SourceWatch {
				kind = store.KindWatch
			}
			msg, err := tx.InsertMessage(ctx, store.Message{
				UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, Role: store.RoleAssistant,
				Kind: kind, Content: message, APIContent: apiContent, Visible: true,
				Unread: run.Source == store.SourceWatch, CreatedAtMs: now,
			})
			if err != nil {
				return err
			}
			if payload.Answer != nil {
				payload.Answer.MessageID = msg.ID
				if err := tx.SavePresentation(ctx, *payload.Answer); err != nil {
					return err
				}
				if _, err := AppendEvent(ctx, tx, nil, run, store.EventAnswerCommitted, store.EventPayload{Answer: payload.Answer, Text: message}); err != nil {
					return err
				}
			}
			if err := tx.InsertOutbox(ctx, store.Outbox{
				UserID: run.UserID, MessageID: msg.ID, Op: store.IndexOpUpsert,
				PayloadJSON: string(mustJSON(map[string]any{
					"userId": run.UserID, "sessionId": run.SessionID, "messageId": msg.ID,
					"role": store.RoleAssistant, "content": message, "createdAtMs": now,
				})), CreatedAtMs: now,
			}); err != nil {
				return err
			}
			thread.LastMessageID = msg.ID
			thread.LastMessagePreview = store.Preview(message, 80)
			thread.LastMessageAtMs = now
			if run.Source == store.SourceWatch {
				thread.UnreadCount++
			}
		}
		if thread.ActiveRunID == run.ID {
			thread.ActiveRunID = 0
		}
		thread.UpdatedAtMs = now
		if err := tx.SaveThread(ctx, *thread); err != nil {
			return err
		}
		if run.Source == store.SourceUser && status == store.StatusDone {
			session, err := tx.GetSession(ctx, run.SessionID)
			if err != nil {
				return err
			}
			session.SuccessfulUserTurns++
			if err := tx.UpdateSession(ctx, *session); err != nil {
				return err
			}
			if session.SuccessfulUserTurns%10 == 0 {
				if _, err := tx.InsertRun(ctx, store.Run{
					UserID: run.UserID, SessionID: run.SessionID, RequestID: "review-" + itoa(now),
					Source: store.SourceMemoryReview, Status: store.StatusQueued, Phase: store.PhaseQueued,
					Priority: store.PriorityMemoryReview, ConsentVersion: run.ConsentVersion, InputVersion: 1,
					PromptEpoch: session.PromptEpoch, CreatedAtMs: now, LastActivityAtMs: now,
				}); err != nil {
					return err
				}
			}
		}
		if run.Source == store.SourceWatch {
			bucketID := watchBucketID(run.QueuedPayload)
			if bucketID > 0 {
				if err := tx.FinishWatchDelivery(ctx, bucketID, run.UserID, run.ID, status, now); err != nil {
					return err
				}
			}
		}
		_, err = AppendEvent(ctx, tx, nil, run, eventType, payload)
		return err
	})
	if err != nil {
		return err
	}
	if e.Notify != nil {
		_ = e.Notify.Wake(ctx, run.ID)
	}
	return nil
}

func watchBucketID(payload []byte) int64 {
	var parsed struct {
		BucketID int64 `json:"bucket_id"`
	}
	if json.Unmarshal(payload, &parsed) != nil {
		return 0
	}
	return parsed.BucketID
}
