package model

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// Service-context wiring is enforced by deploy.TestSQLServiceContextsDisableParameterLogging.
func TestPrivateMessageSQLLoggingPolicy(t *testing.T) {
	sqlx.DisableLog()
	t.Cleanup(func() { sqlx.SetSlowThreshold(500 * time.Millisecond) })
	for _, scenario := range []struct {
		name  string
		delay time.Duration
		fail  bool
	}{
		{name: "success"},
		{name: "slow", delay: time.Millisecond},
		{name: "failure", fail: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			sqlx.SetSlowThreshold(500 * time.Millisecond)
			if scenario.delay > 0 {
				sqlx.SetSlowThreshold(time.Nanosecond)
			}
			const privateText = "SYNTHETIC_PRIVATE_MESSAGE_DO_NOT_LOG"
			var logs bytes.Buffer
			previous := logx.Reset()
			logx.SetWriter(logx.NewWriter(&logs))
			t.Cleanup(func() {
				logx.Reset()
				if previous != nil {
					logx.SetWriter(previous)
				}
			})
			var writeErr error
			if scenario.fail {
				writeErr = errors.New("synthetic driver error: " + privateText)
			}
			db := sql.OpenDB(privacyConnector{delay: scenario.delay, writeErr: writeErr})
			t.Cleanup(func() { _ = db.Close() })
			commands := NewMessageCommandModel(sqlx.NewSqlConnFromDB(db))
			result, err := commands.CreateMessageWithConversations(context.Background(), 7, 8, privateText, 1, 0, "privacy-test")
			if scenario.fail {
				if !errors.Is(err, writeErr) {
					t.Fatalf("expected original storage failure, got %v", err)
				}
			} else if err != nil || !result.Created {
				t.Fatalf("message write failed: result=%+v err=%v", result, err)
			}
			if strings.Contains(logs.String(), privateText) {
				t.Fatal("private message content appeared in SQL logs")
			}
		})
	}
}

type privacyConnector struct {
	delay    time.Duration
	writeErr error
}

func (c privacyConnector) Connect(context.Context) (driver.Conn, error) {
	return privacyConn{privacyConnector: c}, nil
}
func (c privacyConnector) Driver() driver.Driver { return privacyDriver{connector: c} }

type privacyDriver struct{ connector privacyConnector }

func (d privacyDriver) Open(string) (driver.Conn, error) {
	return privacyConn{privacyConnector: d.connector}, nil
}

type privacyConn struct{ privacyConnector }

func (privacyConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepared statements not used in this regression")
}
func (privacyConn) Close() error              { return nil }
func (privacyConn) Begin() (driver.Tx, error) { return privacyTx{}, nil }
func (c privacyConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	if c.delay > 0 {
		time.Sleep(c.delay)
	}
	if c.writeErr != nil {
		return nil, c.writeErr
	}
	return privacyResult{}, nil
}
func (privacyConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "from `message`") {
		return &privacyRows{columns: []string{"id", "receiver_id", "content", "msg_type", "media_id"}}, nil
	}
	return &privacyRows{columns: []string{"id"}, values: [][]driver.Value{{int64(1)}}}, nil
}

type privacyTx struct{}

func (privacyTx) Commit() error   { return nil }
func (privacyTx) Rollback() error { return nil }

type privacyResult struct{}

func (privacyResult) LastInsertId() (int64, error) { return 1, nil }
func (privacyResult) RowsAffected() (int64, error) { return 1, nil }

type privacyRows struct {
	columns []string
	values  [][]driver.Value
}

func (r *privacyRows) Columns() []string { return r.columns }
func (*privacyRows) Close() error        { return nil }
func (r *privacyRows) Next(values []driver.Value) error {
	if len(r.values) == 0 {
		return io.EOF
	}
	copy(values, r.values[0])
	r.values = r.values[1:]
	return nil
}
