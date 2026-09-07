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

func (e *Engine) requireConfirm(workCtx, persistCtx context.Context, run *store.Run, call llm.ToolCall, digest string) error {
	targetRevision, err := expectedRevision(call.Arguments)
	if err != nil || targetRevision <= 0 {
		return errx.New(errx.ParamError, "delete_post confirmation requires a concrete revision")
	}
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
			return nil
		}
		if _, err := tx.InsertConfirmation(ctx, store.Confirmation{
			UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, CallID: call.ID, Tool: call.Name,
			CanonicalArgsDigest: digest, TargetRevision: targetRevision,
			Status: store.ConfirmPending, CreatedAtMs: store.NowMs(),
		}); err != nil {
			return err
		}
		created = true
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
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if e.cancelled(persistCtx, run) {
			return errRunCancelled
		}
		conf, err := e.Store.GetConfirmation(persistCtx, run.ID, call.ID)
		if err != nil {
			return err
		}
		if conf != nil && conf.Status == store.ConfirmApproved {
			return nil
		}
		if conf != nil && conf.Status == store.ConfirmRejected {
			return errx.New(errx.PermissionDenied, "delete_post rejected")
		}
		select {
		case <-workCtx.Done():
			if e.cancelled(persistCtx, run) {
				return errRunCancelled
			}
			return workCtx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return errx.New(errx.ParamError, "delete_post confirmation expired")
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
