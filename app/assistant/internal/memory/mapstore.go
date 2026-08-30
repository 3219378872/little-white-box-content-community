package memory

import (
	"context"
	"strings"
	"sync"
	"unicode/utf8"

	"esx/pkg/errx"
)

type MapStore struct {
	mu      sync.Mutex
	next    int64
	entries map[int64]Entry
	changes map[int64]Change
	Scanner Scanner
}

func NewMapStore() *MapStore {
	return &MapStore{next: 1, entries: map[int64]Entry{}, changes: map[int64]Change{}}
}

func (m *MapStore) List(_ context.Context, userID int64, target string) ([]Entry, []Capacity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.activeLocked(userID, target)
	return all, m.capLocked(userID), nil
}

func (m *MapStore) Active(_ context.Context, userID int64) ([]Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeLocked(userID, ""), nil
}

func (m *MapStore) Add(ctx context.Context, userID int64, target, content, requestID string, nowMs int64) (Entry, int64, error) {
	entries, ids, err := m.Batch(ctx, userID, requestID, []Op{{Op: OpAdd, Target: target, Content: content}}, nowMs)
	if err != nil {
		return Entry{}, 0, err
	}
	changeID := int64(0)
	if len(ids) > 0 {
		changeID = ids[0]
	}
	if len(entries) == 0 {
		return Entry{}, changeID, nil
	}
	return entries[0], changeID, nil
}

func (m *MapStore) Replace(ctx context.Context, userID, id int64, content string, version int32, requestID string, nowMs int64) (Entry, int64, error) {
	entries, ids, err := m.Batch(ctx, userID, requestID, []Op{{Op: OpReplace, ID: id, Content: content, Version: version}}, nowMs)
	if err != nil {
		return Entry{}, 0, err
	}
	changeID := int64(0)
	if len(ids) > 0 {
		changeID = ids[0]
	}
	if len(entries) == 0 {
		return Entry{}, changeID, nil
	}
	return entries[0], changeID, nil
}

func (m *MapStore) Remove(ctx context.Context, userID, id int64, version int32, requestID string, nowMs int64) (int64, error) {
	_, ids, err := m.Batch(ctx, userID, requestID, []Op{{Op: OpRemove, ID: id, Version: version}}, nowMs)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return ids[0], nil
}

func (m *MapStore) Batch(ctx context.Context, userID int64, requestID string, ops []Op, nowMs int64) ([]Entry, []int64, error) {
	if userID <= 0 {
		return nil, nil, errx.NewWithCode(errx.LoginRequired)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := make([]Entry, 0)
	ids := make([]int64, 0)
	for _, op := range ops {
		entry, changeID, err := m.applyLocked(ctx, userID, requestID, op, nowMs)
		if err != nil {
			return nil, nil, err
		}
		if entry != nil {
			entries = append(entries, *entry)
		}
		if changeID > 0 {
			ids = append(ids, changeID)
		}
	}
	return entries, ids, nil
}

func (m *MapStore) applyLocked(ctx context.Context, userID int64, requestID string, op Op, nowMs int64) (*Entry, int64, error) {
	if requestID != "" && requestID != "anon" {
		for _, change := range m.changes {
			if change.UserID != userID || change.RequestID != requestID {
				continue
			}
			if !memoryReplayMatches(op, change) {
				return nil, 0, errx.NewWithCode(errx.IdempotencyConflict)
			}
			if strings.EqualFold(strings.TrimSpace(op.Op), OpRemove) {
				return nil, change.ID, nil
			}
			if change.After == nil {
				return nil, 0, errx.NewWithCode(errx.IdempotencyConflict)
			}
			entry := *change.After
			return &entry, change.ID, nil
		}
	}
	switch strings.ToLower(strings.TrimSpace(op.Op)) {
	case OpAdd, "":
		if !ValidTarget(op.Target) {
			return nil, 0, errx.New(errx.ParamError, "memory target must be memory or user")
		}
		content := strings.TrimSpace(op.Content)
		if err := ScanContent(ctx, m.Scanner, content); err != nil {
			return nil, 0, err
		}
		norm := Normalize(content)
		for _, existing := range m.entries {
			if existing.UserID == userID && existing.Target == op.Target && !existing.Deleted && Normalize(existing.Content) == norm {
				cp := existing
				return &cp, 0, nil
			}
		}
		all := m.activeLocked(userID, op.Target)
		if UsedRunes(all, op.Target)+utf8.RuneCountInString(content) > LimitFor(op.Target) {
			return nil, 0, errx.New(errx.ParamError, "memory capacity exceeded")
		}
		id := m.next
		m.next++
		entry := Entry{ID: id, UserID: userID, Target: op.Target, Content: content, Version: 1, CreatedAtMs: nowMs, UpdatedAtMs: nowMs}
		m.entries[id] = entry
		changeID := m.recordLocked(userID, id, OpAdd, nil, &entry, 1, requestID, nowMs)
		return &entry, changeID, nil
	case OpReplace:
		current, ok := m.entries[op.ID]
		if !ok || current.UserID != userID || current.Deleted {
			return nil, 0, errx.NewWithCode(errx.NotFound)
		}
		if current.Version != op.Version {
			cp := current
			return &cp, 0, errx.New(errx.ContentVersionConflict, "memory version conflict")
		}
		content := strings.TrimSpace(op.Content)
		if err := ScanContent(ctx, m.Scanner, content); err != nil {
			return nil, 0, err
		}
		all := m.activeLocked(userID, current.Target)
		used := UsedRunes(all, current.Target) - utf8.RuneCountInString(current.Content) + utf8.RuneCountInString(content)
		if used > LimitFor(current.Target) {
			return nil, 0, errx.New(errx.ParamError, "memory capacity exceeded")
		}
		before := current
		current.Content = content
		current.Version++
		current.UpdatedAtMs = nowMs
		m.entries[op.ID] = current
		changeID := m.recordLocked(userID, op.ID, OpReplace, &before, &current, current.Version, requestID, nowMs)
		return &current, changeID, nil
	case OpRemove:
		current, ok := m.entries[op.ID]
		if !ok || current.UserID != userID || current.Deleted {
			return nil, 0, errx.NewWithCode(errx.NotFound)
		}
		if current.Version != op.Version {
			return nil, 0, errx.New(errx.ContentVersionConflict, "memory version conflict")
		}
		before := current
		current.Deleted = true
		current.Version++
		current.UpdatedAtMs = nowMs
		m.entries[op.ID] = current
		changeID := m.recordLocked(userID, op.ID, OpRemove, &before, &current, current.Version, requestID, nowMs)
		return &current, changeID, nil
	default:
		return nil, 0, errx.New(errx.ParamError, "unknown memory op")
	}
}

func (m *MapStore) Undo(_ context.Context, userID, changeID int64, nowMs int64) (*Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	change, ok := m.changes[changeID]
	if !ok || change.UserID != userID {
		return nil, errx.NewWithCode(errx.NotFound)
	}
	if change.Undone {
		return nil, errx.New(errx.ContentVersionConflict, "memory change already undone")
	}
	current, ok := m.entries[change.EntryID]
	if !ok || current.UserID != userID {
		return nil, errx.NewWithCode(errx.NotFound)
	}
	if current.Version != change.ResultVersion {
		return nil, errx.New(errx.ContentVersionConflict, "memory version conflict")
	}
	switch change.Op {
	case OpAdd:
		current.Deleted = true
		current.Version++
		current.UpdatedAtMs = nowMs
		m.entries[current.ID] = current
	case OpReplace, OpRemove:
		if change.Before == nil {
			return nil, errx.NewWithCode(errx.SystemError)
		}
		restored := *change.Before
		restored.Deleted = false
		restored.Version = current.Version + 1
		restored.UpdatedAtMs = nowMs
		m.entries[restored.ID] = restored
		current = restored
	}
	change.Undone = true
	m.changes[changeID] = change
	return &current, nil
}

func (m *MapStore) RecordFeedback(_ context.Context, _ int64, _ string, _ int64, _ string) error {
	return nil
}

func (m *MapStore) activeLocked(userID int64, target string) []Entry {
	out := make([]Entry, 0)
	for _, entry := range m.entries {
		if entry.UserID != userID || entry.Deleted {
			continue
		}
		if target != "" && entry.Target != target {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func (m *MapStore) capLocked(userID int64) []Capacity {
	all := m.activeLocked(userID, "")
	return []Capacity{
		{Target: TargetMemory, Used: UsedRunes(all, TargetMemory), Limit: CapacityMemory},
		{Target: TargetUser, Used: UsedRunes(all, TargetUser), Limit: CapacityUser},
	}
}

func (m *MapStore) recordLocked(userID, entryID int64, op string, before, after *Entry, version int32, requestID string, nowMs int64) int64 {
	id := m.next
	m.next++
	change := Change{ID: id, UserID: userID, EntryID: entryID, Op: op, ResultVersion: version, RequestID: requestID, CreatedAtMs: nowMs}
	if before != nil {
		cp := *before
		change.Before = &cp
	}
	if after != nil {
		cp := *after
		change.After = &cp
	}
	m.changes[id] = change
	return id
}
