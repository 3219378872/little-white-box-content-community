package model

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"esx/pkg/event"
	"esx/pkg/outboxx"
	"esx/pkg/sqlstore"

	"github.com/stretchr/testify/require"
)

type snapshotDriver struct{ state *snapshotState }
type snapshotState struct {
	committed  bool
	rolledBack bool
	queries    []string
	payload    []byte
	failOutbox bool
}
type snapshotConn struct{ s *snapshotState }
type snapshotRows struct{ done bool }
type snapshotResult struct{}

func (snapshotResult) LastInsertId() (int64, error)        { return 11, nil }
func (snapshotResult) RowsAffected() (int64, error)        { return 1, nil }
func (d *snapshotDriver) Open(string) (driver.Conn, error) { return &snapshotConn{s: d.state}, nil }
func (c *snapshotConn) Close() error                       { return nil }
func (c *snapshotConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *snapshotConn) Begin() (driver.Tx, error) { return c, nil }
func (c *snapshotConn) Commit() error             { c.s.committed = true; return nil }
func (c *snapshotConn) Rollback() error           { c.s.rolledBack = true; return nil }
func (c *snapshotConn) ExecContext(_ context.Context, q string, a []driver.NamedValue) (driver.Result, error) {
	c.s.queries = append(c.s.queries, q)
	if strings.Contains(q, "INSERT INTO event_outbox") {
		if c.s.failOutbox {
			return nil, errors.New("outbox failed")
		}
		c.s.payload = append([]byte(nil), a[4].Value.([]byte)...)
	}
	return snapshotResult{}, nil
}
func (c *snapshotConn) QueryContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(q, "SELECT revision, like_count, favorite_count FROM action_count") {
		return nil, fmt.Errorf("unexpected query %s", q)
	}
	return &snapshotRows{}, nil
}
func (r *snapshotRows) Columns() []string {
	return []string{"revision", "like_count", "favorite_count"}
}
func (r *snapshotRows) Close() error { return nil }
func (r *snapshotRows) Next(v []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	v[0], v[1], v[2] = int64(42), int64(5), int64(2)
	return nil
}

var snapshotDriverID atomic.Int64

func TestInteractionCommandsEnqueueLockedCountSnapshot(t *testing.T) {
	for _, operation := range []string{"like", "unlike", "favorite", "unfavorite"} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fail=%t", operation, fail), func(t *testing.T) {
				state := &snapshotState{failOutbox: fail}
				name := fmt.Sprintf("snapshot-%d", snapshotDriverID.Add(1))
				sql.Register(name, &snapshotDriver{state})
				db, err := sql.Open(name, "")
				require.NoError(t, err)
				defer db.Close()
				conn := sqlstore.NewSqlConnFromDB(db)
				m := NewInteractionCommandModel(conn, outboxx.NewSQLStore(conn))
				out := outboxx.Event{ID: 10, Topic: "user-behavior-v2", Key: "55", Payload: []byte(`{"action":"like","other_field":"preserved"}`)}
				switch operation {
				case "like":
					_, err = m.Like(context.Background(), 7, 55, 1, out)
				case "unlike":
					err = m.Unlike(context.Background(), 11, 55, 1, out)
				case "favorite":
					_, err = m.Favorite(context.Background(), 7, 55, out)
				case "unfavorite":
					err = m.Unfavorite(context.Background(), 11, 55, out)
				}
				if fail {
					require.Error(t, err)
					require.True(t, state.rolledBack)
					require.False(t, state.committed)
					return
				}
				require.NoError(t, err)
				require.True(t, state.committed)
				var payload struct {
					Snapshot event.InteractionCountSnapshot `json:"count_snapshot"`
					Other    string                         `json:"other_field"`
				}
				require.NoError(t, json.Unmarshal(state.payload, &payload))
				require.Equal(t, int64(42), payload.Snapshot.Revision)
				require.Equal(t, int64(5), payload.Snapshot.LikeCount)
				require.Equal(t, int64(2), payload.Snapshot.FavoriteCount)
				require.Equal(t, "preserved", payload.Other)
				require.Contains(t, strings.Join(state.queries, "\n"), "revision = revision + 1")
			})
		}
	}
}
