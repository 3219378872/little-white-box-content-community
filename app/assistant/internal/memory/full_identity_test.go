package memory

import (
	"context"
	"database/sql"
	sqlx "esx/pkg/sqlstore"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

type identitySQLDB struct {
	rows    []Entry
	norms   []string
	changes int64
	lookup  string
}
type identitySQLResult int64

func (r identitySQLResult) LastInsertId() (int64, error) { return int64(r), nil }
func (r identitySQLResult) RowsAffected() (int64, error) { return 1, nil }
func (d *identitySQLDB) RawDB() (*sql.DB, error) {
	return nil, fmt.Errorf("synthetic SQL interface, no server")
}
func (d *identitySQLDB) PrepareCtx(context.Context, string) (sqlx.StmtSession, error) {
	return nil, fmt.Errorf("unexpected prepare")
}
func (d *identitySQLDB) TransactCtx(ctx context.Context, f func(context.Context, sqlx.Session) error) error {
	return f(ctx, d)
}
func (d *identitySQLDB) ExecCtx(_ context.Context, q string, args ...any) (sql.Result, error) {
	switch {
	case strings.Contains(q, "INSERT INTO memory_target_lock"):
		return identitySQLResult(0), nil
	case strings.Contains(q, "INSERT INTO core_memory_entry"):
		e := Entry{ID: int64(len(d.rows) + 1), UserID: args[0].(int64), Target: args[1].(string), Content: args[2].(string), Version: 1, CreatedAtMs: args[4].(int64), UpdatedAtMs: args[5].(int64)}
		d.rows = append(d.rows, e)
		d.norms = append(d.norms, args[3].(string))
		return identitySQLResult(e.ID), nil
	case strings.Contains(q, "INSERT INTO memory_change"):
		d.changes++
		return identitySQLResult(d.changes), nil
	default:
		return nil, fmt.Errorf("unexpected exec %s", q)
	}
}
func fillIdentityRow(v reflect.Value, e Entry) {
	for k, x := range map[string]any{"ID": e.ID, "UserID": e.UserID, "Target": e.Target, "Content": e.Content, "Version": int64(e.Version), "CreatedAtMs": e.CreatedAtMs, "UpdatedAtMs": e.UpdatedAtMs} {
		v.FieldByName(k).Set(reflect.ValueOf(x))
	}
}
func (d *identitySQLDB) QueryRowCtx(_ context.Context, v any, q string, args ...any) error {
	if strings.Contains(q, "FROM memory_change") {
		return sqlx.ErrNotFound
	}
	if !strings.Contains(q, "content_norm=?") {
		return fmt.Errorf("unexpected row %s", q)
	}
	d.lookup = args[2].(string)
	for i, e := range d.rows {
		if e.UserID == args[0].(int64) && e.Target == args[1].(string) && d.norms[i] == d.lookup {
			fillIdentityRow(reflect.ValueOf(v).Elem(), e)
			return nil
		}
	}
	return sqlx.ErrNotFound
}
func (d *identitySQLDB) QueryRowsCtx(_ context.Context, v any, q string, args ...any) error {
	if !strings.Contains(q, "FROM core_memory_entry") {
		return fmt.Errorf("unexpected rows %s", q)
	}
	out := reflect.ValueOf(v).Elem()
	out.Set(reflect.MakeSlice(out.Type(), 0, len(d.rows)))
	for _, e := range d.rows {
		if e.UserID != args[0].(int64) || len(args) > 1 && e.Target != args[1].(string) {
			continue
		}
		row := reflect.New(out.Type().Elem()).Elem()
		fillIdentityRow(row, e)
		out.Set(reflect.Append(out, row))
	}
	return nil
}

func TestMemoryFullNormalizedIdentity(t *testing.T) {
	for _, target := range []string{TargetMemory, TargetUser} {
		for _, kind := range []string{"map", "sql-interface"} {
			t.Run(target+"/"+kind, func(t *testing.T) {
				var st Store = NewMapStore()
				if kind == "sql-interface" {
					st = NewSQLStore(&identitySQLDB{}, nil)
				}
				memoryAddIdentityContract(t, st, target)
			})
		}
	}
}

func memoryAddIdentityContract(t *testing.T, st Store, target string) {
	t.Helper()
	ctx := context.Background()
	prefix := strings.Repeat("甲", 512)
	one, two := prefix+"已确定", prefix+"尚未确定"
	if utf8.RuneCountInString(one)+utf8.RuneCountInString(two) > LimitFor(target) {
		t.Fatal("fixture exceeds capacity")
	}
	first, _, err := st.Add(ctx, 1, target, one, "first", 1)
	if err != nil {
		t.Fatal(err)
	}
	second, change, err := st.Add(ctx, 1, target, two, "second", 2)
	if err != nil || second.ID == first.ID || change == 0 || second.Content != two {
		t.Fatalf("distinct identity lost: first=%+v second=%+v change=%d err=%v", first, second, change, err)
	}
	// The matching row need not be the first common-prefix candidate.
	duplicate, change, err := st.Add(ctx, 1, target, "  "+two+"  ", "second-duplicate", 3)
	if err != nil || duplicate.ID != second.ID || change != 0 {
		t.Fatalf("duplicate=%+v change=%d err=%v", duplicate, change, err)
	}
	for i, text := range []string{"café", "cafe", "Case \t spacing", "CASE   SPACING", "é", "e\u0301"} {
		entry, change, err := st.Add(ctx, 2, target, text, fmt.Sprintf("unicode-%d", i), 4)
		if err != nil {
			t.Fatal(err)
		}
		if i == 3 {
			if change != 0 {
				t.Fatal("normalized whitespace/case not deduplicated")
			}
		} else if change == 0 || entry.ID == 0 {
			t.Fatalf("SQL collation conflated distinct Go identity %q", text)
		}
	}
}

func TestMapStoreBatchAtomicity(t *testing.T) {
	t.Run("two_valid_ops", func(t *testing.T) {
		m := NewMapStore()
		_, _, err := m.Batch(context.Background(), 1, "batch", []Op{{Op: OpAdd, Target: TargetMemory, Content: "first"}, {Op: OpAdd, Target: TargetMemory, Content: "second"}}, 1)
		active, _ := m.Active(context.Background(), 1)
		t.Logf("two valid operations: err=%v active=%+v", err, active)
		if err != nil || len(active) != 2 {
			t.Fatal("MapStore reuses one request ID for distinct batch items")
		}
	})
	t.Run("rollback_on_failure", func(t *testing.T) {
		m := NewMapStore()
		_, _, err := m.Batch(context.Background(), 1, "", []Op{{Op: OpAdd, Target: TargetMemory, Content: "first"}, {Op: OpAdd, Target: "invalid", Content: "second"}}, 1)
		active, _ := m.Active(context.Background(), 1)
		t.Logf("failed second operation: err=%v active=%+v", err, active)
		if err == nil || len(active) != 0 {
			t.Fatal("failed MapStore batch keeps first item")
		}
	})
}
