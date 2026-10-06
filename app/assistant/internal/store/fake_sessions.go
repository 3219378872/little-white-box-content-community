package store

import (
	"context"

	sqlx "esx/pkg/sqlstore"
)

// LockThread is the in-memory Store.LockThread used by unit tests.
func (m *MemoryStore) LockThread(_ context.Context, userID int64) (*Thread, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	thread, ok := m.threads[userID]
	if !ok {
		thread = Thread{UserID: userID, UpdatedAtMs: NowMs()}
		m.threads[userID] = thread
	}
	cp := thread
	return &cp, nil
}

// GetThread is the in-memory Store.GetThread used by unit tests.
func (m *MemoryStore) GetThread(_ context.Context, userID int64) (*Thread, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	thread, ok := m.threads[userID]
	if !ok {
		return &Thread{UserID: userID}, nil
	}
	cp := thread
	return &cp, nil
}

// SaveThread is the in-memory Store.SaveThread used by unit tests.
func (m *MemoryStore) SaveThread(_ context.Context, thread Thread) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.threads[thread.UserID] = thread
	return nil
}

// CreateSession is the in-memory Store.CreateSession used by unit tests.
func (m *MemoryStore) CreateSession(_ context.Context, session Session) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session.ID = m.nextID()
	m.sessions[session.ID] = session
	return session, nil
}

// GetSession is the in-memory Store.GetSession used by unit tests.
func (m *MemoryStore) GetSession(_ context.Context, id int64) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[id]
	if !ok {
		return nil, sqlx.ErrNotFound
	}
	cp := session
	return &cp, nil
}

// UpdateSession is the in-memory Store.UpdateSession used by unit tests.
func (m *MemoryStore) UpdateSession(_ context.Context, session Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.ID] = session
	return nil
}

// CloseSession is the in-memory Store.CloseSession used by unit tests.
func (m *MemoryStore) CloseSession(_ context.Context, id int64, closedAtMs int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[id]
	if !ok {
		return sqlx.ErrNotFound
	}
	session.Status = SessionClosed
	session.ClosedAtMs = closedAtMs
	m.sessions[id] = session
	return nil
}

// ClearSessionHistory is the in-memory Store.ClearSessionHistory used by unit tests.
func (m *MemoryStore) ClearSessionHistory(_ context.Context, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, session := range m.sessions {
		if session.UserID == userID {
			session.PromptEpoch++
			session.PromptSnapshot = nil
			session.CompactSummary = ""
			session.SuccessfulUserTurns = 0
			m.sessions[id] = session
		}
	}
	return nil
}
