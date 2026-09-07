package runtime

import (
	"context"
	"esx/app/assistant/internal/store"
	"time"
)

func watchCancel(persistCtx context.Context, st store.Store, claimed store.Run, cancelWork context.CancelFunc) func() {
	if st == nil {
		return func() {}
	}
	watchCtx, stop := context.WithCancel(persistCtx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(cancelWatchInterval)
		defer ticker.Stop()
		check := func() bool {
			run, err := st.GetRun(watchCtx, claimed.ID)
			if err == nil && run != nil && run.CancelRequested {
				return true
			}
			version, granted, consentErr := st.AgentConsent(watchCtx, claimed.UserID)
			if consentErr != nil {
				return true
			}
			if !granted || version != claimed.ConsentVersion {
				_ = st.RequestCancel(watchCtx, claimed.UserID, claimed.ID)
				return true
			}
			return false
		}
		if check() {
			cancelWork()
			return
		}
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
				if check() {
					cancelWork()
					return
				}
			}
		}
	}()
	return func() {
		stop()
		<-done
	}
}

func (e *Engine) cancelled(persistCtx context.Context, run *store.Run) bool {
	if run == nil || e.Store == nil {
		return false
	}
	fresh, err := e.Store.GetRun(persistCtx, run.ID)
	if err != nil {
		run.CancelRequested = true
		return true
	}
	if fresh == nil {
		return run.CancelRequested
	}
	if fresh.CancelRequested {
		run.CancelRequested = true
		return true
	}
	return run.CancelRequested
}

func (e *Engine) abortIfRequested(persistCtx context.Context, run store.Run) (bool, error) {
	if !e.cancelled(persistCtx, &run) {
		return false, nil
	}
	return true, e.cancel(persistCtx, run)
}

func (e *Engine) requireFrozenConsent(ctx context.Context, run *store.Run) error {
	if run == nil {
		return nil
	}
	version, granted, err := e.Store.AgentConsent(ctx, run.UserID)
	if err != nil {
		return err
	}
	if !granted || version <= 0 || version != run.ConsentVersion {
		run.CancelRequested = true
		return errRunCancelled
	}
	return nil
}
