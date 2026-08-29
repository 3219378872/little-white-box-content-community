package store

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// MemoryStore is an in-memory Store for unit tests.
type MemoryStore struct {
	mu          sync.Mutex
	next        int64
	threads     map[int64]Thread
	sessions    map[int64]Session
	messages    map[int64]Message
	runs        map[int64]Run
	events      map[int64][]Event
	toolCalls   map[string]ToolCall
	journals    map[string]Journal
	sources     map[string]Source
	confirms    map[string]Confirmation
	queue       map[int64][]QueueItem
	alerts      map[string]Alert
	outbox      []Outbox
	buckets     map[int64]DeliveryBucket
	bucketByKey map[string]int64
	sent        map[string]int
	claimFail   bool
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		next:        1,
		threads:     map[int64]Thread{},
		sessions:    map[int64]Session{},
		messages:    map[int64]Message{},
		runs:        map[int64]Run{},
		events:      map[int64][]Event{},
		toolCalls:   map[string]ToolCall{},
		journals:    map[string]Journal{},
		sources:     map[string]Source{},
		confirms:    map[string]Confirmation{},
		queue:       map[int64][]QueueItem{},
		alerts:      map[string]Alert{},
		buckets:     map[int64]DeliveryBucket{},
		bucketByKey: map[string]int64{},
		sent:        map[string]int{},
	}
}

func (m *MemoryStore) nextID() int64 {
	id := m.next
	m.next++
	return id
}

func (m *MemoryStore) Transact(ctx context.Context, fn func(ctx context.Context, tx Store) error) error {
	return fn(ctx, m)
}

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

func (m *MemoryStore) InsertMessage(_ context.Context, msg Message) (Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg.ID = m.nextID()
	m.messages[msg.ID] = msg
	return msg, nil
}

func (m *MemoryStore) GetMessage(_ context.Context, userID, id int64) (*Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg, ok := m.messages[id]
	if !ok || msg.UserID != userID {
		return nil, sqlx.ErrNotFound
	}
	cp := msg
	return &cp, nil
}

func (m *MemoryStore) ListMessages(_ context.Context, userID, sessionID, afterID int64, limit int) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	out := make([]Message, 0)
	for _, msg := range m.messages {
		if msg.UserID != userID || msg.DeletedAtMs != 0 || !msg.Visible {
			continue
		}
		if sessionID > 0 && msg.SessionID != sessionID {
			continue
		}
		if msg.ID <= afterID {
			continue
		}
		out = append(out, msg)
	}
	sortMessages(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryStore) ListSessionMessages(_ context.Context, userID, sessionID int64, includeHidden bool) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Message, 0)
	for _, msg := range m.messages {
		if msg.UserID != userID || msg.SessionID != sessionID || msg.DeletedAtMs != 0 {
			continue
		}
		if !includeHidden && !msg.Visible {
			continue
		}
		out = append(out, msg)
	}
	sortMessages(out)
	return out, nil
}

func (m *MemoryStore) GetMessagesByIDs(_ context.Context, userID int64, ids []int64) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Message, 0, len(ids))
	for _, id := range ids {
		msg, ok := m.messages[id]
		if ok && msg.UserID == userID {
			out = append(out, msg)
		}
	}
	return out, nil
}

func (m *MemoryStore) SoftDeleteMessages(_ context.Context, userID, deletedAtMs int64) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]int64, 0)
	for id, msg := range m.messages {
		if msg.UserID == userID && msg.DeletedAtMs == 0 {
			msg.DeletedAtMs = deletedAtMs
			m.messages[id] = msg
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (m *MemoryStore) MarkMessagesRead(_ context.Context, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, msg := range m.messages {
		if msg.UserID == userID {
			msg.Unread = false
			m.messages[id] = msg
		}
	}
	return nil
}

func (m *MemoryStore) MarkMessagesCompacted(_ context.Context, ids []int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[int64]struct{}{}
	for _, id := range ids {
		want[id] = struct{}{}
	}
	for id, msg := range m.messages {
		if _, ok := want[id]; ok {
			msg.Compacted = true
			m.messages[id] = msg
		}
	}
	return nil
}

func (m *MemoryStore) InsertRun(_ context.Context, run Run) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run.ID = m.nextID()
	m.runs[run.ID] = run
	return run, nil
}

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

func (m *MemoryStore) GetRunByRequestID(_ context.Context, userID int64, requestID string) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, run := range m.runs {
		if run.UserID == userID && run.RequestID == requestID {
			cp := run
			return &cp, nil
		}
	}
	return nil, sqlx.ErrNotFound
}

func (m *MemoryStore) UpdateRun(_ context.Context, run Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[run.ID] = run
	return nil
}

func (m *MemoryStore) RequestCancel(_ context.Context, userID, runID int64) error {
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

func (m *MemoryStore) Claim(_ context.Context, owner string, nowMs, leaseMs int64) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.claimFail {
		return nil, nil
	}
	var best *Run
	for _, run := range m.runs {
		if run.Status != StatusQueued && !(run.Status == StatusRunning && (run.LeaseUntilMs == 0 || run.LeaseUntilMs < nowMs)) {
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
	best.LeaseUntilMs = nowMs + leaseMs
	best.HeartbeatAtMs = nowMs
	best.LastActivityAtMs = nowMs
	m.runs[best.ID] = *best
	return best, nil
}

func (m *MemoryStore) RenewLease(_ context.Context, runID int64, owner string, leaseUntilMs, heartbeatMs int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[runID]
	if !ok || run.LeaseOwner != owner || run.Status != StatusRunning {
		return false, nil
	}
	run.LeaseUntilMs = leaseUntilMs
	run.HeartbeatAtMs = heartbeatMs
	m.runs[runID] = run
	return true, nil
}

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

func (m *MemoryStore) InsertEvent(_ context.Context, runID int64, eventType string, payload []byte, createdAtMs int64) (Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seq := int64(len(m.events[runID]) + 1)
	ev := Event{ID: m.nextID(), RunID: runID, Seq: seq, Type: eventType, PayloadJSON: payload, CreatedAtMs: createdAtMs}
	m.events[runID] = append(m.events[runID], ev)
	return ev, nil
}

func (m *MemoryStore) ListEventsAfter(_ context.Context, runID, afterSeq int64) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Event, 0)
	for _, ev := range m.events[runID] {
		if ev.Seq > afterSeq {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (m *MemoryStore) MaxEventSeq(_ context.Context, runID int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int64(len(m.events[runID])), nil
}

func (m *MemoryStore) InsertToolCall(_ context.Context, call ToolCall) (ToolCall, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	call.ID = m.nextID()
	m.toolCalls[toolKey(call.RunID, call.CallID)] = call
	return call, nil
}

func (m *MemoryStore) GetToolCall(_ context.Context, runID int64, callID string) (*ToolCall, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	call, ok := m.toolCalls[toolKey(runID, callID)]
	if !ok {
		return nil, sqlx.ErrNotFound
	}
	cp := call
	return &cp, nil
}

func (m *MemoryStore) UpdateToolCall(_ context.Context, call ToolCall) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolCalls[toolKey(call.RunID, call.CallID)] = call
	return nil
}

func (m *MemoryStore) ListToolCalls(_ context.Context, runID int64) ([]ToolCall, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ToolCall, 0)
	for _, call := range m.toolCalls {
		if call.RunID == runID {
			out = append(out, call)
		}
	}
	return out, nil
}

func (m *MemoryStore) GetJournal(_ context.Context, userID int64, requestID, tool, digest string) (*Journal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.journals[journalKey(userID, requestID, tool, digest)]
	if !ok {
		return nil, nil
	}
	cp := row
	return &cp, nil
}

func (m *MemoryStore) ReserveJournal(_ context.Context, row Journal) (*Journal, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := journalKey(row.UserID, row.RequestID, row.Tool, row.CanonicalArgsDigest)
	if existing, ok := m.journals[key]; ok {
		cp := existing
		return &cp, false, nil
	}
	row.ID = m.nextID()
	m.journals[key] = row
	return &row, true, nil
}

func (m *MemoryStore) CompleteJournal(_ context.Context, id int64, status, resultJSON string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, row := range m.journals {
		if row.ID == id {
			row.Status = status
			row.ResultJSON = resultJSON
			m.journals[key] = row
			return nil
		}
	}
	return sqlx.ErrNotFound
}

func (m *MemoryStore) ListSuccessfulJournal(_ context.Context, userID int64, requestID string) ([]Journal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Journal, 0)
	for _, row := range m.journals {
		if row.UserID == userID && row.RequestID == requestID && row.Status == JournalSuccess {
			out = append(out, row)
		}
	}
	return out, nil
}

func (m *MemoryStore) InsertSource(_ context.Context, src Source) (Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	src.ID = m.nextID()
	m.sources[sourceKey(src.RunID, src.Handle)] = src
	return src, nil
}

func (m *MemoryStore) GetSources(_ context.Context, runID int64, handles []string) ([]Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Source, 0)
	for _, handle := range handles {
		if src, ok := m.sources[sourceKey(runID, handle)]; ok {
			out = append(out, src)
		}
	}
	return out, nil
}

func (m *MemoryStore) ListSources(_ context.Context, runID int64) ([]Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Source, 0)
	for _, src := range m.sources {
		if src.RunID == runID {
			out = append(out, src)
		}
	}
	return out, nil
}

func (m *MemoryStore) InsertConfirmation(_ context.Context, row Confirmation) (Confirmation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row.ID = m.nextID()
	m.confirms[confirmKey(row.RunID, row.CallID)] = row
	return row, nil
}

func (m *MemoryStore) GetConfirmation(_ context.Context, runID int64, callID string) (*Confirmation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.confirms[confirmKey(runID, callID)]
	if !ok {
		return nil, nil
	}
	cp := row
	return &cp, nil
}

func (m *MemoryStore) ResolveConfirmation(_ context.Context, userID, runID int64, callID, digest string, approved bool, nowMs int64) (*Confirmation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := confirmKey(runID, callID)
	row, ok := m.confirms[key]
	if !ok {
		return nil, nil
	}
	if row.UserID != userID || row.CanonicalArgsDigest != digest || row.Status != ConfirmPending {
		cp := row
		return &cp, nil
	}
	if approved {
		row.Status = ConfirmApproved
	} else {
		row.Status = ConfirmRejected
	}
	row.ResolvedAtMs = nowMs
	m.confirms[key] = row
	cp := row
	return &cp, nil
}

func (m *MemoryStore) CountQueue(_ context.Context, runID int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.queue[runID]), nil
}

func (m *MemoryStore) Enqueue(_ context.Context, item QueueItem) (QueueItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item.ID = m.nextID()
	m.queue[item.RunID] = append(m.queue[item.RunID], item)
	return item, nil
}

func (m *MemoryStore) ListQueue(_ context.Context, runID int64) ([]QueueItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]QueueItem(nil), m.queue[runID]...)
	return out, nil
}

func (m *MemoryStore) DeleteQueue(_ context.Context, runID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.queue, runID)
	return nil
}

func (m *MemoryStore) InsertAlert(_ context.Context, alert Alert) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := alertKey(alert.RunID, alert.Level, alert.Dimension)
	if _, ok := m.alerts[key]; ok {
		return false, nil
	}
	m.alerts[key] = alert
	return true, nil
}

func (m *MemoryStore) InsertOutbox(_ context.Context, row Outbox) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	row.ID = m.nextID()
	m.outbox = append(m.outbox, row)
	return nil
}

func (m *MemoryStore) ListUnpublishedOutbox(_ context.Context, limit int) ([]Outbox, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Outbox, 0)
	for _, row := range m.outbox {
		if !row.Published {
			out = append(out, row)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *MemoryStore) MarkOutboxPublished(_ context.Context, ids []int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[int64]struct{}{}
	for _, id := range ids {
		want[id] = struct{}{}
	}
	for i, row := range m.outbox {
		if _, ok := want[row.ID]; ok {
			row.Published = true
			m.outbox[i] = row
		}
	}
	return nil
}

func (m *MemoryStore) UpsertDeliveryBucket(_ context.Context, userID, hitID, windowStartMs, nowMs int64) (DeliveryBucket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := bucketKey(userID, windowStartMs)
	if id, ok := m.bucketByKey[key]; ok {
		b := m.buckets[id]
		b.HitIDs = append(b.HitIDs, hitID)
		m.buckets[id] = b
		return b, nil
	}
	id := m.nextID()
	b := DeliveryBucket{ID: id, UserID: userID, WindowStartMs: windowStartMs, Status: "pending", HitIDs: []int64{hitID}, CreatedAtMs: nowMs}
	m.buckets[id] = b
	m.bucketByKey[key] = id
	return b, nil
}

func (m *MemoryStore) GetBucket(_ context.Context, id int64) (*DeliveryBucket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.buckets[id]
	if !ok {
		return nil, nil
	}
	cp := b
	return &cp, nil
}

func (m *MemoryStore) ListDueBuckets(_ context.Context, nowMs, windowMs int64) ([]DeliveryBucket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]DeliveryBucket, 0)
	for _, b := range m.buckets {
		if (b.Status == "pending" || b.Status == "deferred") && b.WindowStartMs+windowMs <= nowMs {
			out = append(out, b)
		}
	}
	return out, nil
}

func (m *MemoryStore) GetPendingBucket(_ context.Context, userID int64) (*DeliveryBucket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var best *DeliveryBucket
	for _, b := range m.buckets {
		if b.UserID != userID || (b.Status != "pending" && b.Status != "deferred") {
			continue
		}
		cp := b
		if best == nil || cp.WindowStartMs < best.WindowStartMs {
			best = &cp
		}
	}
	return best, nil
}

func (m *MemoryStore) MarkBucketScheduled(_ context.Context, id, runID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.buckets[id]
	b.Status = "scheduled"
	b.RunID = runID
	m.buckets[id] = b
	return nil
}

func (m *MemoryStore) MarkBucketSent(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.buckets[id]
	b.Status = "sent"
	m.buckets[id] = b
	return nil
}

func (m *MemoryStore) ResetUnsentBuckets(_ context.Context, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, b := range m.buckets {
		if b.UserID == userID && b.Status == "scheduled" {
			b.Status = "pending"
			b.RunID = 0
			m.buckets[id] = b
		}
	}
	return nil
}

func (m *MemoryStore) CountSent(_ context.Context, userID, taskID int64, periodKind string, periodStartMs int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sent[sentKey(userID, taskID, periodKind, periodStartMs)], nil
}

func (m *MemoryStore) IncrSent(_ context.Context, userID, taskID int64, periodKind string, periodStartMs int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent[sentKey(userID, taskID, periodKind, periodStartMs)]++
	return nil
}

func sortMessages(msgs []Message) {
	for i := 0; i < len(msgs); i++ {
		for j := i + 1; j < len(msgs); j++ {
			if msgs[j].ID < msgs[i].ID {
				msgs[i], msgs[j] = msgs[j], msgs[i]
			}
		}
	}
}

func toolKey(runID int64, callID string) string {
	return itoa(runID) + ":" + callID
}
func journalKey(userID int64, requestID, tool, digest string) string {
	return itoa(userID) + ":" + requestID + ":" + tool + ":" + digest
}
func sourceKey(runID int64, handle string) string { return itoa(runID) + ":" + handle }
func confirmKey(runID int64, callID string) string {
	return itoa(runID) + ":" + callID
}
func alertKey(runID int64, level, dim string) string {
	return itoa(runID) + ":" + level + ":" + dim
}
func bucketKey(userID, window int64) string { return itoa(userID) + ":" + itoa(window) }
func sentKey(userID, taskID int64, kind string, start int64) string {
	return itoa(userID) + ":" + itoa(taskID) + ":" + kind + ":" + itoa(start)
}
func itoa(v int64) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

type MemoryNotifier struct {
	mu    sync.Mutex
	token map[int64]int64
}

func NewMemoryNotifier() *MemoryNotifier {
	return &MemoryNotifier{token: map[int64]int64{}}
}

func (n *MemoryNotifier) Wake(_ context.Context, runID int64) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.token[runID]++
	return nil
}

func (n *MemoryNotifier) WakeToken(_ context.Context, runID int64) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return itoa(n.token[runID]), nil
}
