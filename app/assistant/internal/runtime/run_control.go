package runtime

import (
	"context"
	"esx/app/assistant/internal/store"
	"time"
)

// watchCancel 周期检查 run 是否被取消、换租或撤销授权，命中时取消 workCtx；授权失效时顺带标记取消。
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
			if err == nil && run != nil && (run.Fence() != claimed.Fence() || run.CancelRequested) {
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

// ownedRun 重新读取 run，并确认租约仍属于本 worker。
func (e *Engine) ownedRun(ctx context.Context, claimed store.Run) (*store.Run, error) {
	run, err := e.Store.GetRun(ctx, claimed.ID)
	if err != nil {
		return nil, err
	}
	if run == nil || run.Fence() != claimed.Fence() {
		return nil, store.ErrLeaseLost
	}
	return run, nil
}

// cancelled 判断 run 是否已请求取消；读库失败时按已取消处理，宁可停下也不越权继续。
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

// abortIfRequested 在已请求取消时把 run 收尾为 cancelled。
func (e *Engine) abortIfRequested(persistCtx context.Context, run store.Run) (bool, error) {
	if !e.cancelled(persistCtx, &run) {
		return false, nil
	}
	return true, e.cancel(persistCtx, run)
}

// requireFrozenConsent 确认用户授权仍有效且版本与 run 冻结时一致，否则按取消处理。
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
