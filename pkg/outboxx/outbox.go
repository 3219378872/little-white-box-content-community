package outboxx

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
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

// sqlRecord stays flat because go-zero's SQL mapper does not populate fields
// nested in an embedded struct.
type sqlRecord struct {
	ID        int64  `db:"id"`
	Topic     string `db:"topic"`
	Tag       string `db:"tag"`
	Key       string `db:"message_key"`
	Payload   []byte `db:"payload"`
	Attempts  int    `db:"attempts"`
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
}

type SQLStore struct {
	conn sqlx.SqlConn
}

func NewSQLStore(conn sqlx.SqlConn) *SQLStore {
	return &SQLStore{conn: conn}
}

func (s *SQLStore) Enqueue(ctx context.Context, session sqlx.Session, event Event) error {
	if session == nil {
		return fmt.Errorf("outboxx: nil transaction session")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	_, err := session.ExecCtx(ctx, enqueueSQL,
		event.ID, event.Topic, event.Tag, event.Key, event.Payload,
		StatusPending, now, now, now,
	)
	return err
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

func (s *SQLStore) Claim(
	ctx context.Context,
	owner string,
	limit int,
	now time.Time,
	lease time.Duration,
) ([]Record, error) {
	if s == nil || s.conn == nil {
		return nil, fmt.Errorf("outboxx: nil store connection")
	}
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
	var rows []sqlRecord
	err := s.conn.QueryRowsCtx(ctx, &rows, `SELECT
        id, topic, tag, message_key, payload, attempts, created_at
    FROM event_outbox
    WHERE ((status IN (?, ?) AND next_attempt_at <= ?)
        OR (status = ? AND locked_until <= ?))
    ORDER BY id
    LIMIT ?`, StatusPending, StatusRetry, nowMillis, StatusProcessing, nowMillis, limit)
	if err != nil {
		return nil, err
	}

	claimed := make([]Record, 0, len(rows))
	lockedUntil := now.Add(lease).UnixMilli()
	for _, row := range rows {
		result, execErr := s.conn.ExecCtx(ctx, `UPDATE event_outbox
            SET status = ?, attempts = attempts + 1, locked_by = ?, locked_until = ?, updated_at = ?
            WHERE id = ? AND ((status IN (?, ?) AND next_attempt_at <= ?)
                OR (status = ? AND locked_until <= ?))`,
			StatusProcessing, owner, lockedUntil, nowMillis, row.ID,
			StatusPending, StatusRetry, nowMillis, StatusProcessing, nowMillis,
		)
		if execErr != nil {
			return nil, execErr
		}
		affected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return nil, rowsErr
		}
		if affected != 1 {
			continue
		}
		claimed = append(claimed, Record{
			Event: Event{
				ID: row.ID, Topic: row.Topic, Tag: row.Tag, Key: row.Key, Payload: row.Payload,
			},
			Attempts:  row.Attempts + 1,
			LockedBy:  owner,
			CreatedAt: row.CreatedAt,
		})
	}
	return claimed, nil
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
