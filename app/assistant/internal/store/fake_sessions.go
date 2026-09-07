package store

import (
	"context"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

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

func (m *MemoryStore) SaveThread(_ context.Context, thread Thread) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.threads[thread.UserID] = thread
	return nil
}

func (m *MemoryStore) CreateSession(_ context.Context, session Session) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session.ID = m.nextID()
	m.sessions[session.ID] = session
	return session, nil
}

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

func (m *MemoryStore) UpdateSession(_ context.Context, session Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.ID] = session
	return nil
}

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
