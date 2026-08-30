package runtime

import (
	"context"
	"time"

	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
)

// ColdConversationIdle is the visible-thread idle that makes the next new run
// rebuild Safety/SOUL/tool rules/MEMORY. It is not the per-run HardIdle budget.
const ColdConversationIdle = 30 * time.Minute

func isColdConversation(thread *store.Thread, now int64) bool {
	if thread == nil || thread.LastMessageAtMs <= 0 {
		return false
	}
	return now-thread.LastMessageAtMs >= ColdConversationIdle.Milliseconds()
}

func loadActiveMemory(ctx context.Context, memories memory.Store, userID int64) []memory.Entry {
	if memories == nil {
		return nil
	}
	listed, err := memories.Active(ctx, userID)
	if err != nil {
		return nil
	}
	return listed
}

func encodePersistentSnapshot(entries []memory.Entry, compactSummary string) []byte {
	return prompt.EncodeSnapshot(prompt.BuildSnapshot(entries, nil, compactSummary))
}

func ensureForegroundSession(ctx context.Context, tx store.Store, memories memory.Store, thread *store.Thread, now int64) (*store.Session, error) {
	if thread.SessionID > 0 {
		session, err := tx.GetSession(ctx, thread.SessionID)
		if err == nil && session != nil {
			if session.Status == store.SessionClosed {
				session.Status = store.SessionOpen
				session.ClosedAtMs = 0
				if len(session.PromptSnapshot) == 0 {
					if session.PromptEpoch < 1 {
						session.PromptEpoch = 1
					}
					session.PromptSnapshot = encodePersistentSnapshot(loadActiveMemory(ctx, memories, thread.UserID), session.CompactSummary)
				}
				if err := tx.UpdateSession(ctx, *session); err != nil {
					return nil, err
				}
			}
			thread.SessionID = session.ID
			return session, nil
		}
	}
	created, err := tx.CreateSession(ctx, store.Session{
		UserID: thread.UserID, PromptEpoch: 1,
		PromptSnapshot: encodePersistentSnapshot(loadActiveMemory(ctx, memories, thread.UserID), ""),
		Status:         store.SessionOpen, CreatedAtMs: now,
	})
	if err != nil {
		return nil, err
	}
	thread.SessionID = created.ID
	return &created, nil
}

func spliceColdSession(ctx context.Context, tx store.Store, memories memory.Store, session *store.Session) (*store.Session, error) {
	if session == nil {
		return nil, nil
	}
	session.PromptEpoch++
	session.PromptSnapshot = encodePersistentSnapshot(loadActiveMemory(ctx, memories, session.UserID), session.CompactSummary)
	session.Status = store.SessionOpen
	session.ClosedAtMs = 0
	if err := tx.UpdateSession(ctx, *session); err != nil {
		return nil, err
	}
	return session, nil
}

func spliceIfCold(ctx context.Context, tx store.Store, memories memory.Store, thread *store.Thread, session *store.Session, now int64) (*store.Session, error) {
	if session == nil || thread == nil || thread.ActiveRunID > 0 || !isColdConversation(thread, now) {
		return session, nil
	}
	return spliceColdSession(ctx, tx, memories, session)
}
