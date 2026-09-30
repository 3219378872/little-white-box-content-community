package runtime

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"esx/app/assistant/internal/store"
	sqlx "esx/pkg/sqlstore"
)

// Exercises SQLStore's real transaction and run-lock path without presenting
// this SQL-interface contract as evidence of live MySQL concurrency.
type modelResultSQLContract struct {
	inTx, locked bool
	version      int64
	cancelled    int64
}

func (s *modelResultSQLContract) TransactCtx(ctx context.Context, fn func(context.Context, sqlx.Session) error) error {
	s.inTx = true
	defer func() { s.inTx = false; s.locked = false }()
	return fn(ctx, s)
}
func (s *modelResultSQLContract) QueryRowCtx(_ context.Context, out any, q string, args ...any) error {
	if !s.inTx {
		return errors.New("read outside transaction")
	}
	row := reflect.ValueOf(out).Elem()
	if strings.Contains(q, "SELECT id FROM agent_run") {
		if !strings.Contains(q, "FOR UPDATE") || !strings.Contains(q, "lease_generation=?") {
			return errors.New("missing lease row lock")
		}
		s.locked = true
		row.FieldByName("ID").SetInt(1)
		return nil
	}
	if !s.locked {
		return errors.New("input identity read before lease lock")
	}
	row.FieldByName("ID").SetInt(1)
	row.FieldByName("InputVersion").SetInt(s.version)
	row.FieldByName("CancelRequested").SetInt(s.cancelled)
	return nil
}
func (*modelResultSQLContract) QueryRowsCtx(context.Context, any, string, ...any) error {
	return errors.New("unexpected rows")
}
func (*modelResultSQLContract) ExecCtx(context.Context, string, ...any) (sql.Result, error) {
	return nil, errors.New("unexpected write")
}
func (*modelResultSQLContract) PrepareCtx(context.Context, string) (sqlx.StmtSession, error) {
	return nil, errors.New("unexpected prepare")
}
func (*modelResultSQLContract) RawDB() (*sql.DB, error) {
	return nil, errors.New("SQL interface fixture has no server")
}

func TestModelResultSQLCommitChecksIdentityUnderLeaseLock(t *testing.T) {
	for _, tc := range []struct {
		name               string
		version, cancelled int64
		want               error
	}{
		{"current", 1, 0, nil}, {"redirected", 2, 0, errRunRedirected}, {"cancelled", 1, 1, errRunCancelled}, {"cancel_wins", 2, 1, errRunCancelled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := &modelResultSQLContract{version: tc.version, cancelled: tc.cancelled}
			e := &Engine{Store: store.NewSQLStore(db)}
			called := false
			err := e.modelResultStep(context.Background(), store.Run{ID: 1, LeaseOwner: "worker", LeaseGeneration: 1, InputVersion: 1}, func(context.Context, store.Store) error {
				called = true
				if !db.locked || !db.inTx {
					t.Fatal("commit outside lock")
				}
				return nil
			})
			if !errors.Is(err, tc.want) || called != (tc.want == nil) {
				t.Fatalf("called=%v err=%v want=%v", called, err, tc.want)
			}
		})
	}
}
