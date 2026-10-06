package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	sqlx "esx/pkg/sqlstore"

	"github.com/go-sql-driver/mysql"
)

// scriptedExec fails every write with execErr and answers reads from a script, then ErrNotFound.
type scriptedExec struct {
	execErr error
	reads   []error
	readN   int
}

// ExecCtx returns the scripted write error.
func (e *scriptedExec) ExecCtx(context.Context, string, ...any) (sql.Result, error) {
	return nil, e.execErr
}

// QueryRowCtx returns the next scripted read result, leaving v zero-valued.
func (e *scriptedExec) QueryRowCtx(context.Context, any, string, ...any) error {
	defer func() { e.readN++ }()
	if e.readN < len(e.reads) {
		return e.reads[e.readN]
	}
	return sqlx.ErrNotFound
}

// QueryRowsCtx is unused by the paths under test.
func (e *scriptedExec) QueryRowsCtx(context.Context, any, string, ...any) error { return nil }

// errDuplicateWording is not a MySQL 1062 error, only text that happens to say "duplicate".
var errDuplicateWording = errors.New("driver: bad connection after duplicate packet")

func TestLockThreadOnlyRereadsOnDuplicateKey(t *testing.T) {
	ctx := context.Background()
	exec := &scriptedExec{execErr: errDuplicateWording}
	if _, err := (&SQLStore{exec: exec}).LockThread(ctx, 1); !errors.Is(err, errDuplicateWording) {
		t.Fatalf("non-1062 insert error must surface, got %v", err)
	}
	if exec.readN != 1 {
		t.Fatalf("non-1062 insert error must not re-read, reads=%d", exec.readN)
	}

	exec = &scriptedExec{execErr: &mysql.MySQLError{Number: 1062}, reads: []error{sqlx.ErrNotFound, nil}}
	if _, err := (&SQLStore{exec: exec}).LockThread(ctx, 1); err != nil {
		t.Fatalf("1062 must re-read the concurrent row, got %v", err)
	}
}

func TestReserveJournalOnlyTreatsDuplicateKeyAsConflict(t *testing.T) {
	ctx := context.Background()
	row := Journal{UserID: 1, RequestID: "r", Tool: "create_post", CanonicalArgsDigest: "d"}
	exec := &scriptedExec{execErr: errDuplicateWording}
	if _, reserved, err := (&SQLStore{exec: exec}).ReserveJournal(ctx, row); !errors.Is(err, errDuplicateWording) || reserved {
		t.Fatalf("non-1062 insert error must surface, reserved=%v err=%v", reserved, err)
	}

	exec = &scriptedExec{execErr: &mysql.MySQLError{Number: 1062}, reads: []error{sqlx.ErrNotFound, nil}}
	existing, reserved, err := (&SQLStore{exec: exec}).ReserveJournal(ctx, row)
	if err != nil || reserved || existing == nil {
		t.Fatalf("1062 must report the winning entry, existing=%+v reserved=%v err=%v", existing, reserved, err)
	}
}
