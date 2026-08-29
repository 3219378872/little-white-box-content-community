package lease

import (
	"context"
	"time"

	"esx/app/assistant/internal/store"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	DefaultLease = 60 * time.Second
	DefaultRenew = 10 * time.Second
	DefaultOwner = "assistant-agent"
)

type Manager struct {
	Store store.Store
	Owner string
	Lease time.Duration
	Renew time.Duration
}

func (m *Manager) owner() string {
	if m.Owner == "" {
		return DefaultOwner
	}
	return m.Owner
}

func (m *Manager) leaseMs() int64 {
	if m.Lease <= 0 {
		return DefaultLease.Milliseconds()
	}
	return m.Lease.Milliseconds()
}

func (m *Manager) Claim(ctx context.Context) (*store.Run, bool, error) {
	now := store.NowMs()
	run, err := m.Store.Claim(ctx, m.owner(), now, m.leaseMs())
	if err != nil {
		return nil, false, err
	}
	if run == nil {
		return nil, false, nil
	}
	recovered := run.LeaseOwner != "" && run.StartedAtMs > 0 && run.StartedAtMs < now
	if recovered {
		logx.WithContext(ctx).Infow("assistant-agent recovered expired lease",
			logx.Field("runId", run.ID), logx.Field("userId", run.UserID))
	}
	return run, recovered, nil
}

func (m *Manager) RenewLoop(ctx context.Context, runID int64) {
	interval := m.Renew
	if interval <= 0 {
		interval = DefaultRenew
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ok, err := m.Store.RenewLease(ctx, runID, m.owner(), store.NowMs()+m.leaseMs(), store.NowMs())
			if err != nil {
				logx.WithContext(ctx).Errorw("assistant-agent lease renew failed",
					logx.Field("runId", runID), logx.Field("err", err.Error()))
				continue
			}
			if !ok {
				logx.WithContext(ctx).Infow("assistant-agent lost lease", logx.Field("runId", runID))
				return
			}
		}
	}
}
