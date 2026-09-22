package runtime

import (
	"context"
	"encoding/json"
	"esx/app/assistant/internal/canonical"
	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/store"
	"esx/pkg/errx"
	"time"
)

const confirmationWait = 2 * time.Minute

func (e *Engine) requireConfirm(workCtx, persistCtx context.Context, run *store.Run, call llm.ToolCall, digest string) error {
	targetRevision, err := expectedRevision(call.Arguments)
	if err != nil || targetRevision <= 0 {
		return errx.New(errx.ParamError, "delete_post confirmation requires a concrete revision")
	}
	if workCtx.Err() != nil {
		if e.cancelled(persistCtx, run) {
			return errRunCancelled
		}
		return workCtx.Err()
	}
	if e.cancelled(persistCtx, run) {
		return errRunCancelled
	}
	var conf *store.Confirmation
	created := false
	err = e.step(persistCtx, *run, func(ctx context.Context, tx store.Store) error {
		existing, err := tx.GetConfirmation(ctx, run.ID, call.ID)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.UserID != run.UserID || existing.SessionID != run.SessionID ||
				existing.Tool != call.Name || existing.CanonicalArgsDigest != digest || existing.TargetRevision != targetRevision {
				return errx.NewWithCode(errx.PermissionDenied)
			}
			conf = existing
			return nil
		}
		inserted, err := tx.InsertConfirmation(ctx, store.Confirmation{
			UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, CallID: call.ID, Tool: call.Name,
			CanonicalArgsDigest: digest, TargetRevision: targetRevision,
			Status: store.ConfirmPending, CreatedAtMs: store.NowMs(),
		})
		if err != nil {
			return err
		}
		created = true
		conf = &inserted
		_, err = AppendEvent(ctx, tx, nil, *run, store.EventConfirmRequired, store.EventPayload{
			ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: "确认删除帖子", PayloadJSON: call.Arguments},
		})
		return err
	})
	if err != nil {
		return err
	}
	if created && e.Notify != nil {
		_ = e.Notify.Wake(persistCtx, run.ID)
	}
	if conf == nil {
		return errx.New(errx.SystemError, "delete_post confirmation missing")
	}
	switch conf.Status {
	case store.ConfirmApproved:
		return nil
	case store.ConfirmRejected:
		return errx.New(errx.PermissionDenied, "delete_post rejected")
	case store.ConfirmExpired:
		return errx.New(errx.ParamError, "delete_post confirmation expired")
	}
	err = e.step(persistCtx, *run, func(ctx context.Context, tx store.Store) error {
		fresh, err := tx.GetRun(ctx, run.ID)
		if err != nil || fresh == nil {
			return err
		}
		fresh.Status = store.StatusWaitingConfirm
		fresh.Phase = store.PhaseWaitingInput
		fresh.LastActivityAtMs = store.NowMs()
		return tx.UpdateRun(ctx, *fresh)
	})
	if err != nil {
		return err
	}
	run.Status = store.StatusWaitingConfirm
	return errRunWaiting
}

// ExpireConfirmationWaits releases runs parked on delete confirmation.
// Approval and rejection requeue the run; timeout marks the confirmation expired and requeues it
// so the worker loop can claim other runs while a person is looking at the dialog.
func ExpireConfirmationWaits(ctx context.Context, st store.Store, now int64) error {
	if st == nil {
		return nil
	}
	runs, err := st.ListWaitingConfirmRuns(ctx)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if err := expireConfirmationWait(ctx, st, run.ID, now); err != nil {
			return err
		}
	}
	return nil
}

func expireConfirmationWait(ctx context.Context, st store.Store, runID, now int64) error {
	return st.Transact(ctx, func(ctx context.Context, tx store.Store) error {
		run, err := tx.LockRun(ctx, runID)
		if err != nil || run == nil || run.Status != store.StatusWaitingConfirm {
			return err
		}
		pending, err := tx.PendingConfirmation(ctx, run.ID)
		if err != nil {
			return err
		}
		if pending != nil && !run.CancelRequested && now < pending.CreatedAtMs+confirmationWait.Milliseconds() {
			return nil
		}
		if pending != nil && !run.CancelRequested {
			pending.Status = store.ConfirmExpired
			pending.ResolvedAtMs = now
			if err := tx.UpdateConfirmation(ctx, *pending); err != nil {
				return err
			}
		}
		run.Status = store.StatusQueued
		run.Phase = store.PhaseQueued
		run.LastActivityAtMs = now
		return tx.UpdateRun(ctx, *run)
	})
}

func expectedRevision(argsJSON string) (int64, error) {
	var args struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if err := json.Unmarshal([]byte(canonical.UnwrapArgsJSON(argsJSON)), &args); err != nil {
		return 0, err
	}
	return args.ExpectedRevision, nil
}
