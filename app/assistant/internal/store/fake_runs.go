package store

import (
	"context"

	"esx/app/assistant/internal/consent"
	sqlx "esx/pkg/sqlstore"
)

// InsertRun is the in-memory Store.InsertRun used by unit tests.
func (m *MemoryStore) InsertRun(_ context.Context, run Run) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run.ID = m.nextID()
	m.runs[run.ID] = run
	return run, nil
}

// GetRun is the in-memory Store.GetRun used by unit tests.
func (m *MemoryStore) GetRun(_ context.Context, id int64) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[id]
	if !ok {
		return nil, sqlx.ErrNotFound
	}
	cp := run
	return &cp, nil
}

// GetRunByRequestID is the in-memory Store.GetRunByRequestID used by unit tests.
func (m *MemoryStore) GetRunByRequestID(_ context.Context, userID int64, requestID string) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, run := range m.runs {
		if run.UserID == userID && run.RequestID == requestID {
			cp := run
			return &cp, nil
		}
	}
	return nil, nil
}

// UpdateRun is the in-memory Store.UpdateRun used by unit tests.
func (m *MemoryStore) UpdateRun(_ context.Context, run Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.runs[run.ID]; ok {
		run.CancelRequested = run.CancelRequested || existing.CancelRequested
		run.QueuedPayload = append([]byte(nil), existing.QueuedPayload...)
		run.LeaseOwner = existing.LeaseOwner
		run.LeaseGeneration = existing.LeaseGeneration
		run.LeaseUntilMs = existing.LeaseUntilMs
		run.HeartbeatAtMs = existing.HeartbeatAtMs
		run.ConsentVersion = existing.ConsentVersion
		run.InputVersion = existing.InputVersion
	}
	m.runs[run.ID] = run
	return nil
}

// ExpireLease is the in-memory Store.ExpireLease used by unit tests.
func (m *MemoryStore) ExpireLease(runID, leaseUntilMs int64) {
	m.stepMu.Lock()
	defer m.stepMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	run := m.runs[runID]
	run.LeaseUntilMs = leaseUntilMs
	m.runs[runID] = run
}

// SetRunInput is the in-memory Store.SetRunInput used by unit tests.
func (m *MemoryStore) SetRunInput(_ context.Context, runID int64, payload []byte, lastActivityMs int64) error {
	m.stepMu.Lock()
	defer m.stepMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[runID]
	if !ok || (run.Status != StatusRunning && run.Status != StatusQueued) {
		return sqlx.ErrNotFound
	}
	run.QueuedPayload = append([]byte(nil), payload...)
	run.InputVersion++
	run.LastActivityAtMs = lastActivityMs
	m.runs[runID] = run
	return nil
}

// RequestCancel is the in-memory Store.RequestCancel used by unit tests.
func (m *MemoryStore) RequestCancel(_ context.Context, userID, runID int64) error {
	m.stepMu.Lock()
	defer m.stepMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[runID]
	if !ok || run.UserID != userID {
		return sqlx.ErrNotFound
	}
	run.CancelRequested = true
	m.runs[runID] = run
	return nil
}

// RequestCancelAll is the in-memory Store.RequestCancelAll used by unit tests.
func (m *MemoryStore) RequestCancelAll(_ context.Context, userID int64) error {
	m.stepMu.Lock()
	defer m.stepMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, run := range m.runs {
		if run.UserID == userID && !IsTerminalStatus(run.Status) {
			run.CancelRequested = true
			m.runs[id] = run
		}
	}
	return nil
}

// LockOpenRuns is the in-memory Store.LockOpenRuns used by unit tests.
func (m *MemoryStore) LockOpenRuns(_ context.Context, userID int64) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var runs []Run
	for _, run := range m.runs {
		if run.UserID == userID && !IsTerminalStatus(run.Status) {
			runs = append(runs, run)
		}
	}
	return runs, nil
}

// CancelOpenBackground is the in-memory Store.CancelOpenBackground used by unit tests.
func (m *MemoryStore) CancelOpenBackground(_ context.Context, userID int64, sources []string) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[string]struct{}{}
	for _, src := range sources {
		want[src] = struct{}{}
	}
	out := make([]Run, 0)
	for id, run := range m.runs {
		if run.UserID != userID || IsTerminalStatus(run.Status) {
			continue
		}
		if _, ok := want[run.Source]; !ok {
			continue
		}
		run.CancelRequested = true
		m.runs[id] = run
		out = append(out, run)
	}
	return out, nil
}

// Claim mirrors the SQL claim order: priority first, then age.
func (m *MemoryStore) Claim(_ context.Context, owner string, nowMs, leaseMs int64) (*Run, error) {
	m.stepMu.Lock()
	defer m.stepMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.claimFail {
		return nil, nil
	}
	var best *Run
	for _, run := range m.runs {
		claimable := run.Status == StatusQueued ||
			(run.Status == StatusRunning && (run.LeaseUntilMs == 0 || run.LeaseUntilMs < nowMs))
		if !claimable {
			continue
		}
		cp := run
		if best == nil || cp.Priority < best.Priority || (cp.Priority == best.Priority && cp.CreatedAtMs < best.CreatedAtMs) {
			best = &cp
		}
	}
	if best == nil {
		return nil, nil
	}
	best.Status = StatusRunning
	if best.StartedAtMs == 0 {
		best.StartedAtMs = nowMs
	}
	best.LeaseOwner = owner
	best.LeaseGeneration++
	best.LeaseUntilMs = nowMs + leaseMs
	best.HeartbeatAtMs = nowMs
	best.LastActivityAtMs = nowMs
	m.runs[best.ID] = *best
	return best, nil
}

// RenewLease is the in-memory Store.RenewLease used by unit tests.
func (m *MemoryStore) RenewLease(_ context.Context, runID int64, owner string, generation, leaseUntilMs, heartbeatMs int64) (bool, error) {
	m.stepMu.Lock()
	defer m.stepMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[runID]
	if !ok || run.LeaseOwner != owner || run.LeaseGeneration != generation || run.Status != StatusRunning || run.LeaseUntilMs < heartbeatMs {
		return false, nil
	}
	run.LeaseUntilMs = leaseUntilMs
	run.HeartbeatAtMs = heartbeatMs
	m.runs[runID] = run
	return true, nil
}

// AgentConsent is the in-memory Store.AgentConsent used by unit tests.
func (m *MemoryStore) AgentConsent(_ context.Context, userID int64) (int32, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	version, ok := m.consents[userID]
	if !ok {
		return consent.CurrentVersion, true, nil
	}
	return version, version > 0, nil
}

// SetAgentConsent is the in-memory Store.SetAgentConsent used by unit tests.
func (m *MemoryStore) SetAgentConsent(userID int64, version int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consents[userID] = version
}

// OldestQueuedAgeMs is the in-memory Store.OldestQueuedAgeMs used by unit tests.
func (m *MemoryStore) OldestQueuedAgeMs(_ context.Context, nowMs int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var oldest int64
	for _, run := range m.runs {
		if run.Status != StatusQueued {
			continue
		}
		if oldest == 0 || run.CreatedAtMs < oldest {
			oldest = run.CreatedAtMs
		}
	}
	if oldest == 0 {
		return 0, nil
	}
	return nowMs - oldest, nil
}
