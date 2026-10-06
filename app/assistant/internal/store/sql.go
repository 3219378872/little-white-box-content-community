package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	sqlx "esx/pkg/sqlstore"
)

// execer is the subset shared by a connection and a transaction session.
type execer interface {
	ExecCtx(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowCtx(ctx context.Context, v any, query string, args ...any) error
	QueryRowsCtx(ctx context.Context, v any, query string, args ...any) error
}

// SQLStore implements Store on MySQL; conn is nil inside a transaction.
type SQLStore struct {
	exec execer
	conn sqlx.SqlConn
}

// NewSQLStore wraps a connection; transactions get a session-bound copy.
func NewSQLStore(conn sqlx.SqlConn) *SQLStore {
	return &SQLStore{exec: conn, conn: conn}
}

// Transact runs fn in a transaction, or inline when already inside one.
func (s *SQLStore) Transact(ctx context.Context, fn func(ctx context.Context, tx Store) error) error {
	if s.conn == nil {
		return fn(ctx, s)
	}
	return s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		return fn(ctx, &SQLStore{exec: session})
	})
}

// RunStep commits fn only while the caller still holds the run lease; the row lock
// also serializes worker commits with input acceptance on the same run.
func (s *SQLStore) RunStep(ctx context.Context, fence LeaseFence, fn func(ctx context.Context, tx Store) error) error {
	if fence.RunID <= 0 || strings.TrimSpace(fence.Owner) == "" || fence.Generation <= 0 {
		return ErrLeaseLost
	}
	return s.Transact(ctx, func(ctx context.Context, tx Store) error {
		sqlTx, ok := tx.(*SQLStore)
		if !ok {
			return fmt.Errorf("run step requires SQL store")
		}
		var row struct {
			ID int64 `db:"id"`
		}
		err := sqlTx.exec.QueryRowCtx(ctx, &row, `SELECT id FROM agent_run
			WHERE id=? AND lease_owner=? AND lease_generation=? AND status='running' AND lease_until_ms>=?
			FOR UPDATE`, fence.RunID, fence.Owner, fence.Generation, NowMs())
		if err == sqlx.ErrNotFound {
			return ErrLeaseLost
		}
		if err != nil {
			return err
		}
		return fn(ctx, tx)
	})
}
