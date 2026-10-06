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

// 用户拒绝或确认过期时的工具调用 outcome；与 success/invalid/unavailable 并列写入调用行与 tool_result 事件。
const (
	confirmOutcomeRejected = "rejected"
	confirmOutcomeExpired  = "expired"
)

// confirmationDeclined 表示删除确认被拒绝或已过期：删除绝不执行，说明作为工具结果交还模型继续收尾。
type confirmationDeclined struct {
	outcome string
	text    string
}

// Error 返回交给模型的中文说明。
func (d *confirmationDeclined) Error() string { return d.text }

// requireConfirm records a confirmation for a destructive call pinned to its argument digest and
// target revision, then parks the run as waiting_confirm.
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
		conf = &inserted
		_, err = AppendEvent(ctx, tx, *run, store.EventConfirmRequired, store.EventPayload{
			ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: "确认删除帖子", PayloadJSON: call.Arguments},
		})
		return err
	})
	if err != nil {
		return err
	}
	if conf == nil {
		return errx.New(errx.SystemError, "delete_post confirmation missing")
	}
	switch conf.Status {
	case store.ConfirmApproved:
		return nil
	case store.ConfirmRejected:
		return &confirmationDeclined{outcome: confirmOutcomeRejected, text: "用户已拒绝删除该帖子，未执行删除。"}
	case store.ConfirmExpired:
		return &confirmationDeclined{outcome: confirmOutcomeExpired, text: "删除确认已过期，未执行删除。"}
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

// expireConfirmationWait requeues a run waiting for confirmation once it is cancelled or the wait
// times out; on resume the expired confirmation skips the delete and is reported to the model.
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

// expectedRevision reads the revision the destructive call targets, so confirmation is tied to it.
func expectedRevision(argsJSON string) (int64, error) {
	var args struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if err := json.Unmarshal([]byte(canonical.UnwrapArgsJSON(argsJSON)), &args); err != nil {
		return 0, err
	}
	return args.ExpectedRevision, nil
}
