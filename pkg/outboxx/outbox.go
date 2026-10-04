package outboxx

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	sqlx "esx/pkg/sqlstore"
)

const (
	StatusPending    int8 = 0
	StatusProcessing int8 = 1
	StatusSent       int8 = 2
	StatusRetry      int8 = 3
	StatusDead       int8 = 4
)

// Event is the durable transport record written in the same transaction as a
// business mutation.
type Event struct {
	ID      int64  `db:"id"`
	Topic   string `db:"topic"`
	Tag     string `db:"tag"`
	Key     string `db:"message_key"`
	Payload []byte `db:"payload"`
}

func (e Event) Validate() error {
	if e.ID <= 0 {
		return fmt.Errorf("outboxx: id is required")
	}
	if strings.TrimSpace(e.Topic) == "" {
		return fmt.Errorf("outboxx: topic is required")
	}
	if strings.TrimSpace(e.Key) == "" {
		return fmt.Errorf("outboxx: message key is required")
	}
	if len(e.Payload) == 0 {
		return fmt.Errorf("outboxx: payload is required")
	}
	return nil
}

// Record is an event leased by a relay worker.
type Record struct {
	Event
	Attempts  int    `db:"attempts"`
	LockedBy  string `db:"locked_by"`
	CreatedAt int64  `db:"created_at"`
}

type Backlog struct {
	Count           int64 `db:"count"`
	OldestCreatedAt int64 `db:"oldest_created_at"`
}

// Store is separated from Relay so delivery and recovery behavior can be
// tested without a database or broker.
type Store interface {
	Claim(ctx context.Context, owner string, limit int, now time.Time, lease time.Duration) ([]Record, error)
	MarkSent(ctx context.Context, id int64, owner string, sentAt time.Time) error
	MarkRetry(ctx context.Context, id int64, owner string, attempts, maxAttempts int, nextAttempt time.Time, cause error) error
	Backlog(ctx context.Context) (Backlog, error)
	Purge(ctx context.Context, createdBefore time.Time, limit int) (int64, error)
}

// Waker is implemented by stores that can signal a relay in the same process
// as soon as an enqueued event commits, so delivery need not wait for a poll.
type Waker interface {
	Wakeups() <-chan struct{}
}

type SQLStore struct {
	conn sqlx.SqlConn
	wake chan struct{}
}

// NewSQLStore returns a store whose committed enqueues wake the relay built on
// the same instance; business models and the relay must share it.
func NewSQLStore(conn sqlx.SqlConn) *SQLStore {
	return &SQLStore{conn: conn, wake: make(chan struct{}, 1)}
}

// Wakeups coalesces commit signals into a single pending wakeup.
func (s *SQLStore) Wakeups() <-chan struct{} {
	return s.wake
}

func (s *SQLStore) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *SQLStore) Enqueue(ctx context.Context, session sqlx.Session, event Event) error {
	if session == nil {
		return fmt.Errorf("outboxx: nil transaction session")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	if _, err := session.ExecCtx(ctx, enqueueSQL,
		event.ID, event.Topic, event.Tag, event.Key, event.Payload,
		StatusPending, now, now, now,
	); err != nil {
		return err
	}
	if s.wake != nil && !sqlx.AfterCommit(session, s.notify) {
		// Outside a managed transaction the row is already committed or
		// owned by the caller; an early wakeup is harmless and polling remains.
		s.notify()
	}
	return nil
}

func (s *SQLStore) EnqueueTx(ctx context.Context, tx *sql.Tx, event Event) error {
	if tx == nil {
		return fmt.Errorf("outboxx: nil sql transaction")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	_, err := tx.ExecContext(ctx, enqueueSQL,
		event.ID, event.Topic, event.Tag, event.Key, event.Payload,
		StatusPending, now, now, now,
	)
	return err
}

const enqueueSQL = `INSERT INTO event_outbox
    (id, topic, tag, message_key, payload, status, next_attempt_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

const (
	claimExpiredLeaseSQL = `SELECT id, topic, tag, message_key, payload, attempts, created_at
    FROM event_outbox
    WHERE status = ? AND locked_until <= ?
    ORDER BY locked_until, id
    LIMIT ?
    FOR UPDATE SKIP LOCKED`
	claimDueSQL = `SELECT id, topic, tag, message_key, payload, attempts, created_at
    FROM event_outbox
    WHERE status = ? AND next_attempt_at <= ?
    ORDER BY next_attempt_at, id
    LIMIT ?
    FOR UPDATE SKIP LOCKED`
)

// claimSteps each follow one index order, so LIMIT stops the scan early
// instead of sorting (and locking) the whole backlog. Expired leases and due
// retries go first so a large fresh backlog cannot starve them.
var claimSteps = []struct {
	query  string
	status int8
}{
	{claimExpiredLeaseSQL, StatusProcessing},
	{claimDueSQL, StatusRetry},
	{claimDueSQL, StatusPending},
}

// Claim leases up to limit due events. Concurrent relays skip rows another
// relay has locked rather than contending on the same head of the queue.
func (s *SQLStore) Claim(
	ctx context.Context,
	owner string,
	limit int,
	now time.Time,
	lease time.Duration,
) ([]Record, error) {
	if strings.TrimSpace(owner) == "" {
		return nil, fmt.Errorf("outboxx: owner is required")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("outboxx: claim limit must be positive")
	}
	if lease <= 0 {
		return nil, fmt.Errorf("outboxx: lease must be positive")
	}

	nowMillis := now.UnixMilli()
	lockedUntil := now.Add(lease).UnixMilli()
	var claimed []Record
	err := s.readCommitted(ctx, func(tx *sql.Tx) error {
		for _, step := range claimSteps {
			remaining := limit - len(claimed)
			if remaining <= 0 {
				break
			}
			rows, err := selectClaimable(ctx, tx, step.query, step.status, nowMillis, remaining)
			if err != nil {
				return err
			}
			claimed = append(claimed, rows...)
		}
		if len(claimed) == 0 {
			return nil
		}

		args := make([]any, 0, 4+len(claimed))
		args = append(args, StatusProcessing, owner, lockedUntil, nowMillis)
		for _, record := range claimed {
			args = append(args, record.ID)
		}
		result, err := tx.ExecContext(ctx, `UPDATE event_outbox
            SET status = ?, attempts = attempts + 1, locked_by = ?, locked_until = ?, updated_at = ?
            WHERE id IN (?`+strings.Repeat(", ?", len(claimed)-1)+`)`, args...)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != int64(len(claimed)) {
			return fmt.Errorf("outboxx: claimed %d events but leased %d", len(claimed), affected)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for index := range claimed {
		claimed[index].Attempts++
		claimed[index].LockedBy = owner
	}
	return claimed, nil
}

func selectClaimable(
	ctx context.Context,
	tx *sql.Tx,
	query string,
	status int8,
	nowMillis int64,
	limit int,
) ([]Record, error) {
	rows, err := tx.QueryContext(ctx, query, status, nowMillis, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var records []Record
	for rows.Next() {
		var record Record
		if err := rows.Scan(
			&record.ID, &record.Topic, &record.Tag, &record.Key, &record.Payload,
			&record.Attempts, &record.CreatedAt,
		); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// readCommitted runs fn in a READ COMMITTED transaction: relay scans then lock
// only matching rows and take no gap locks that would block business inserts.
func (s *SQLStore) readCommitted(ctx context.Context, fn func(*sql.Tx) error) error {
	if s == nil || s.conn == nil {
		return fmt.Errorf("outboxx: nil store connection")
	}
	db, err := s.conn.RawDB()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLStore) MarkSent(ctx context.Context, id int64, owner string, sentAt time.Time) error {
	result, err := s.conn.ExecCtx(ctx, `UPDATE event_outbox
        SET status = ?, sent_at = ?, locked_by = '', locked_until = 0, last_error = '', updated_at = ?
        WHERE id = ? AND status = ? AND locked_by = ?`,
		StatusSent, sentAt.UnixMilli(), sentAt.UnixMilli(), id, StatusProcessing, owner,
	)
	if err != nil {
		return err
	}
	return expectOneRow(result, "mark sent", id)
}

func (s *SQLStore) MarkRetry(
	ctx context.Context,
	id int64,
	owner string,
	attempts int,
	maxAttempts int,
	nextAttempt time.Time,
	cause error,
) error {
	status := StatusRetry
	if maxAttempts > 0 && attempts >= maxAttempts {
		status = StatusDead
	}
	message := "unknown publish failure"
	if cause != nil {
		message = cause.Error()
	}
	if len(message) > 1000 {
		message = message[:1000]
	}
	now := time.Now().UnixMilli()
	result, err := s.conn.ExecCtx(ctx, `UPDATE event_outbox
        SET status = ?, next_attempt_at = ?, locked_by = '', locked_until = 0,
            last_error = ?, updated_at = ?
        WHERE id = ? AND status = ? AND locked_by = ?`,
		status, nextAttempt.UnixMilli(), message, now, id, StatusProcessing, owner,
	)
	if err != nil {
		return err
	}
	return expectOneRow(result, "mark retry", id)
}

func (s *SQLStore) Backlog(ctx context.Context) (Backlog, error) {
	var backlog Backlog
	err := s.conn.QueryRowCtx(ctx, &backlog, `SELECT
        count(*) AS count,
        coalesce(min(created_at), 0) AS oldest_created_at
    FROM event_outbox
    WHERE status IN (?, ?, ?)`, StatusPending, StatusProcessing, StatusRetry)
	return backlog, err
}

// Purge deletes up to limit sent events created before createdBefore.
// Pending, retrying and dead events are never removed.
func (s *SQLStore) Purge(ctx context.Context, createdBefore time.Time, limit int) (int64, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("outboxx: purge limit must be positive")
	}
	var deleted int64
	err := s.readCommitted(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `DELETE FROM event_outbox
            WHERE created_at < ? AND status = ?
            LIMIT ?`, createdBefore.UnixMilli(), StatusSent, limit)
		if err != nil {
			return err
		}
		deleted, err = result.RowsAffected()
		return err
	})
	return deleted, err
}

func expectOneRow(result sql.Result, action string, id int64) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("outboxx: %s lost lease for event %d", action, id)
	}
	return nil
}
