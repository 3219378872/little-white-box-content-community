package lease

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"esx/app/assistant/internal/store"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	DefaultLease = 60 * time.Second
	DefaultRenew = 10 * time.Second
	DefaultOwner = "assistant-agent"
)

var ownerSequence atomic.Uint64

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

func NewOwner(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = DefaultOwner
	}
	host, _ := os.Hostname()
	var nonce [6]byte
	nonceText := ""
	if _, err := rand.Read(nonce[:]); err != nil {
		nonceText = fmt.Sprintf("%x", time.Now().UnixNano()+int64(ownerSequence.Add(1)))
	} else {
		nonceText = hex.EncodeToString(nonce[:])
	}
	suffix := fmt.Sprintf("-%d-%s", os.Getpid(), nonceText)
	prefix := base + "-" + host
	if maxPrefix := 64 - len(suffix); len(prefix) > maxPrefix {
		prefix = prefix[:maxPrefix]
	}
	return prefix + suffix
}

func (m *Manager) RenewLoop(ctx context.Context, run store.Run, onLost context.CancelFunc) {
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
			now := store.NowMs()
			ok, err := m.Store.RenewLease(ctx, run.ID, m.owner(), run.LeaseGeneration, now+m.leaseMs(), now)
			if err != nil {
				logx.WithContext(ctx).Errorw("assistant-agent lease renew failed",
					logx.Field("runId", run.ID), logx.Field("err", err.Error()))
				if now >= run.LeaseUntilMs {
					onLost()
					return
				}
				continue
			}
			if !ok {
				logx.WithContext(ctx).Infow("assistant-agent lost lease", logx.Field("runId", run.ID))
				onLost()
				return
			}
			run.LeaseUntilMs = now + m.leaseMs()
		}
	}
}
