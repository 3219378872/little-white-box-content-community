package runtime

import (
	"context"

	"esx/app/assistant/internal/store"
	"esx/pkg/errx"
)

func (a *Acceptor) DeleteHistory(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return errx.NewWithCode(errx.LoginRequired)
	}
	var cancelled []store.Run
	err := a.Store.Transact(ctx, func(ctx context.Context, tx store.Store) error {
		// Match worker and accept lock ordering: runs before thread. Terminalizing
		// under these locks fences old RunStep writers before the prompt is cleared.
		var err error
		cancelled, err = tx.CancelOpenBackground(ctx, userID, []string{store.SourceUser, store.SourceWatch, store.SourceMemoryReview})
		if err != nil {
			return err
		}
		thread, err := tx.LockThread(ctx, userID)
		if err != nil {
			return err
		}
		now := store.NowMs()
		for _, run := range cancelled {
			if err := cancelHistoryRun(ctx, tx, run, now); err != nil {
				return err
			}
		}
		ids, err := tx.SoftDeleteMessages(ctx, userID, now)
		if err != nil {
			return err
		}
		if err := tx.ClearResearchHistory(ctx, userID); err != nil {
			return err
		}
		if err := tx.ClearSessionHistory(ctx, userID); err != nil {
			return err
		}
		for _, id := range ids {
			if err := tx.InsertOutbox(ctx, store.Outbox{UserID: userID, MessageID: id, Op: store.IndexOpDelete, CreatedAtMs: now}); err != nil {
				return err
			}
		}
		thread.ActiveRunID = 0
		thread.LastMessageID = 0
		thread.LastMessagePreview = ""
		thread.LastMessageAtMs = 0
		thread.UnreadCount = 0
		thread.UpdatedAtMs = now
		return tx.SaveThread(ctx, *thread)
	})
	if err == nil && a.Notify != nil {
		for _, run := range cancelled {
			_ = a.Notify.Wake(ctx, run.ID)
		}
	}
	return err
}

func cancelHistoryRun(ctx context.Context, tx store.Store, run store.Run, now int64) error {
	if err := closePendingCalls(ctx, tx, run, store.StatusCancelled); err != nil {
		return err
	}
	confirmation, err := tx.PendingConfirmation(ctx, run.ID)
	if err != nil {
		return err
	}
	if confirmation != nil {
		confirmation.Status = store.ConfirmExpired
		confirmation.ResolvedAtMs = now
		if err := tx.UpdateConfirmation(ctx, *confirmation); err != nil {
			return err
		}
	}
	if err := tx.DeleteQueue(ctx, run.ID); err != nil {
		return err
	}
	if run.Source == store.SourceWatch {
		if bucketID := watchBucketID(run.QueuedPayload); bucketID > 0 {
			if err := tx.FinishWatchDelivery(ctx, bucketID, run.UserID, run.ID, store.StatusCancelled, now); err != nil {
				return err
			}
		}
	}
	_, err = finishRunTx(ctx, tx, run, store.StatusCancelled, store.EventError, store.EventPayload{ErrorCode: "CANCELLED", Text: "run cancelled"}, now)
	return err
}
