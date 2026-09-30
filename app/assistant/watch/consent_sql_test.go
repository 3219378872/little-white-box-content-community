package watch

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"esx/pkg/errx"
	sqlx "esx/pkg/sqlstore"
)

type watchConsentDB struct {
	sqlx.SqlConn
	granted         bool
	version         int32
	readErr         error
	inTx, locked    bool
	writes, commits int
}

func (s *watchConsentDB) TransactCtx(ctx context.Context, fn func(context.Context, sqlx.Session) error) error {
	s.inTx = true
	defer func() { s.inTx = false; s.locked = false }()
	err := fn(ctx, s)
	if err == nil {
		s.commits++
	}
	return err
}
func (s *watchConsentDB) QueryRowCtx(_ context.Context, v any, q string, args ...any) error {
	if !s.inTx {
		return errors.New("query outside transaction")
	}
	value := reflect.ValueOf(v).Elem()
	if strings.Contains(q, "agent_capability_consent") {
		if !strings.Contains(q, "FOR SHARE") || len(args) != 1 || args[0] != int64(1) {
			return errors.New("missing user-scoped consent row lock")
		}
		if s.readErr != nil {
			return s.readErr
		}
		s.locked = true
		var granted int64
		if s.granted {
			granted = 1
		}
		value.FieldByName("Granted").SetInt(granted)
		value.FieldByName("Version").SetInt(int64(s.version))
		return nil
	}
	if !s.locked {
		return errors.New("task read before authorization lock")
	}
	for name, x := range map[string]int64{"ID": 1, "UserID": 1, "Version": 2, "CreatedAt": 1} {
		if f := value.FieldByName(name); f.IsValid() {
			f.SetInt(x)
		}
	}
	return nil
}
func (s *watchConsentDB) ExecCtx(_ context.Context, q string, args ...any) (sql.Result, error) {
	if !s.inTx || !s.locked {
		return nil, errors.New("write without consent transaction lock")
	}
	s.writes++
	return watchMutationResult{}, nil
}

type watchMutationResult struct{}

func (watchMutationResult) LastInsertId() (int64, error) { return 1, nil }
func (watchMutationResult) RowsAffected() (int64, error) { return 1, nil }

type watchConsentReaderFunc func(context.Context, int64) (int32, bool, error)

func (f watchConsentReaderFunc) AgentConsent(ctx context.Context, id int64) (int32, bool, error) {
	return f(ctx, id)
}

func TestSQLWatchMutationConsentIsAtomic(t *testing.T) {
	for _, state := range []string{"current", "missing", "revoked", "old", "future", "failure"} {
		for _, op := range []string{"create", "enable", "disable", "delete"} {
			t.Run(state+"/"+op, func(t *testing.T) {
				db := &watchConsentDB{granted: true, version: 2}
				switch state {
				case "missing":
					db.readErr = sqlx.ErrNotFound
				case "revoked":
					db.granted = false
				case "old":
					db.version = 1
				case "future":
					db.version = 3
				case "failure":
					db.readErr = errors.New("storage failure")
				}
				st := NewSQLStore(db)
				ctx := context.Background()
				var err error
				switch op {
				case "create":
					_, err = st.Create(ctx, Task{UserID: 1, ConditionType: KeywordNewPost, TargetType: "keyword", TargetText: "topic"})
				case "delete":
					err = st.Delete(ctx, 1, 1, 1)
				default:
					_, err = st.UpdateEnabled(ctx, 1, 1, op == "enable", 1)
				}
				if state == "current" {
					if err != nil || db.writes != 1 || db.commits != 1 {
						t.Fatalf("err=%v writes=%d commits=%d", err, db.writes, db.commits)
					}
				} else if err == nil || db.writes != 0 || db.commits != 0 {
					t.Fatalf("denial err=%v writes=%d commits=%d", err, db.writes, db.commits)
				}
			})
		}
	}
}

func TestSQLWatchRechecksRevocationAfterSharedGate(t *testing.T) {
	db := &watchConsentDB{granted: true, version: 2}
	preflight := watchConsentReaderFunc(func(context.Context, int64) (int32, bool, error) { db.granted = false; return 2, true, nil })
	st := WithConsent(NewSQLStore(db), preflight)
	_, err := st.Create(context.Background(), Task{UserID: 1, ConditionType: KeywordNewPost, TargetType: "keyword", TargetText: "topic"})
	if !errx.Is(err, errx.AgentNotAuthorized) || db.writes != 0 {
		t.Fatalf("revoked mutation: err=%v writes=%d", err, db.writes)
	}
}
