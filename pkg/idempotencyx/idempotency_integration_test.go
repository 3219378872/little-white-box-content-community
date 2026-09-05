//go:build integration

package idempotencyx

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type observedSession struct {
	sqlx.Session
	firstLookup chan<- struct{}
}

func (s *observedSession) QueryRowCtx(ctx context.Context, value any, query string, args ...any) error {
	err := s.Session.QueryRowCtx(ctx, value, query, args...)
	if s.firstLookup != nil {
		close(s.firstLookup)
		s.firstLookup = nil
	}
	return err
}

func TestResolveIdempotencySessionSeesConcurrentWinnerUnderRepeatableRead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, err := mysql.Run(ctx,
		"mysql:8.0",
		mysql.WithDatabase("idempotency_test"),
		mysql.WithUsername("root"),
		mysql.WithPassword("testpass"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })

	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	require.NoError(t, err)
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.PingContext(ctx))
	_, err = db.ExecContext(ctx, "CREATE TABLE idempotency ("+
		"id BIGINT NOT NULL PRIMARY KEY,"+
		"scope VARCHAR(64) NOT NULL,"+
		"user_id BIGINT NOT NULL,"+
		"`key` VARCHAR(128) NOT NULL,"+
		"command_hash CHAR(64) NOT NULL,"+
		"resource_id BIGINT NOT NULL,"+
		"created_at BIGINT NOT NULL,"+
		"UNIQUE KEY uniq_scope_user_key (scope, user_id, `key`)"+
		") ENGINE=InnoDB")
	require.NoError(t, err)

	winner, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	record := IdempotencyRecord{
		Scope: "post:create", UserID: 7, Key: "concurrent-key", CommandHash: CommandHash("same-command"),
	}
	_, err = winner.ExecContext(ctx, "INSERT INTO idempotency "+
		"(id, scope, user_id, `key`, command_hash, resource_id, created_at) "+
		"VALUES (?, ?, ?, ?, ?, ?, ?)",
		1, record.Scope, record.UserID, record.Key, record.CommandHash, 101, time.Now().UnixMilli())
	require.NoError(t, err)

	firstLookup := make(chan struct{})
	resolved := make(chan struct {
		resourceID int64
		created    bool
		err        error
	}, 1)
	conn := sqlx.NewSqlConnFromDB(db)
	go func() {
		var result struct {
			resourceID int64
			created    bool
			err        error
		}
		result.err = conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
			result.resourceID, result.created, result.err = ResolveIdempotencySession(
				ctx,
				&observedSession{Session: session, firstLookup: firstLookup},
				record,
				2,
				202,
			)
			return result.err
		})
		resolved <- result
	}()

	select {
	case <-firstLookup:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.NoError(t, winner.Commit())

	select {
	case result := <-resolved:
		require.NoError(t, result.err)
		require.False(t, result.created)
		require.Equal(t, int64(101), result.resourceID)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
