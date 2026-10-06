package memory

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	sqlx "esx/pkg/sqlstore"

	"github.com/go-sql-driver/mysql"
)

// changeInsertSession 让 memory_change 插入以 execErr 失败，并让回查已有记录总能命中。
type changeInsertSession struct {
	execErr error
	reads   int
}

// ExecCtx 返回预设的写入错误。
func (s *changeInsertSession) ExecCtx(context.Context, string, ...any) (sql.Result, error) {
	return nil, s.execErr
}

// QueryRowCtx 模拟回查到已有变更记录。
func (s *changeInsertSession) QueryRowCtx(context.Context, any, string, ...any) error {
	s.reads++
	return nil
}

// QueryRowsCtx 不在被测路径上。
func (s *changeInsertSession) QueryRowsCtx(context.Context, any, string, ...any) error { return nil }

// PrepareCtx 不在被测路径上。
func (s *changeInsertSession) PrepareCtx(context.Context, string) (sqlx.StmtSession, error) {
	return nil, errors.New("unused")
}

func TestInsertChangeOnlyReusesRowOnDuplicateKey(t *testing.T) {
	ctx := context.Background()
	wording := errors.New("driver: bad connection after duplicate packet")
	session := &changeInsertSession{execErr: wording}
	if _, err := (&SQLStore{}).insertChange(ctx, session, 1, 2, OpAdd, nil, &Entry{}, 1, "req", 1); !errors.Is(err, wording) {
		t.Fatalf("non-1062 insert error must surface, got %v", err)
	}
	if session.reads != 0 {
		t.Fatalf("non-1062 insert error must not look up an existing change, reads=%d", session.reads)
	}

	session = &changeInsertSession{execErr: &mysql.MySQLError{Number: 1062}}
	if _, err := (&SQLStore{}).insertChange(ctx, session, 1, 2, OpAdd, nil, &Entry{}, 1, "req", 1); err != nil {
		t.Fatalf("1062 must reuse the concurrent change, got %v", err)
	}
}
