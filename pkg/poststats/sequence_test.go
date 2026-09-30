package poststats

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"esx/pkg/sqlstore"

	"github.com/stretchr/testify/require"
)

type statsState struct {
	mu       sync.Mutex
	snapshot Snapshot
}
type statsDriver struct{ state *statsState }
type statsConn struct {
	state   *statsState
	pending Snapshot
	active  bool
}
type statsRows struct {
	snapshot Snapshot
	done     bool
}

func (d *statsDriver) Open(string) (driver.Conn, error) { return &statsConn{state: d.state}, nil }
func (c *statsConn) Close() error                       { return nil }
func (c *statsConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *statsConn) Begin() (driver.Tx, error) {
	c.state.mu.Lock()
	c.pending = c.state.snapshot
	c.active = true
	return c, nil
}
func (c *statsConn) Commit() error {
	c.state.snapshot = c.pending
	c.active = false
	c.state.mu.Unlock()
	return nil
}
func (c *statsConn) Rollback() error {
	if c.active {
		c.active = false
		c.state.mu.Unlock()
	}
	return nil
}
func (c *statsConn) ExecContext(_ context.Context, q string, a []driver.NamedValue) (driver.Result, error) {
	switch {
	case q == advanceSQL:
		if c.pending.Sequence <= 0 || c.pending.Sequence == 9223372036854775807 {
			return driver.RowsAffected(0), nil
		}
		c.pending.Sequence++
	case strings.Contains(q, "GREATEST(stats_seq"):
		c.pending.Sequence = max(c.pending.Sequence, a[0].Value.(int64))
	case q == "UPDATE post SET like_count = like_count + 1 WHERE id = ?":
		c.pending.LikeCount++
	case q == "UPDATE post SET comment_count = comment_count + 1 WHERE id = ?":
		c.pending.CommentCount++
	default:
		return nil, fmt.Errorf("unexpected SQL %s", q)
	}
	return driver.RowsAffected(1), nil
}
func (c *statsConn) QueryContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
	if q != snapshotSQL {
		return nil, fmt.Errorf("unexpected query %s", q)
	}
	return &statsRows{snapshot: c.pending}, nil
}
func (r *statsRows) Columns() []string { return []string{"like_count", "comment_count", "stats_seq"} }
func (r *statsRows) Close() error      { return nil }
func (r *statsRows) Next(v []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	v[0], v[1], v[2] = r.snapshot.LikeCount, r.snapshot.CommentCount, r.snapshot.Sequence
	return nil
}

var statsDriverNumber atomic.Int64

func statsDB(t *testing.T) (*sql.DB, *statsState) {
	t.Helper()
	s := &statsState{snapshot: Snapshot{Sequence: 9000000000000}}
	name := fmt.Sprintf("poststats-%d", statsDriverNumber.Add(1))
	sql.Register(name, &statsDriver{s})
	db, err := sql.Open(name, "")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db, s
}
func TestSharedSequenceSerializesConcurrentCommentsLikesAndLifecycle(t *testing.T) {
	db, state := statsDB(t)
	conn := sqlstore.NewSqlConnFromDB(db)
	snapshots := make(chan Snapshot, 60)
	failures := make(chan error, 60)
	var wg sync.WaitGroup
	for i := 0; i < 60; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%3 == 0 {
				tx, err := db.BeginTx(context.Background(), nil)
				if err != nil {
					failures <- err
					return
				}
				defer tx.Rollback()
				if _, err = tx.ExecContext(context.Background(), "UPDATE post SET like_count = like_count + 1 WHERE id = ?", 1); err != nil {
					failures <- err
					return
				}
				snapshot, err := AdvanceTx(context.Background(), tx, 1)
				if err == nil {
					err = tx.Commit()
				}
				if err != nil {
					failures <- err
					return
				}
				snapshots <- snapshot
				return
			}
			err := conn.TransactCtx(context.Background(), func(ctx context.Context, session sqlstore.Session) error {
				if i%3 == 1 {
					if _, err := session.ExecCtx(ctx, "UPDATE post SET comment_count = comment_count + 1 WHERE id = ?", 1); err != nil {
						return err
					}
				}
				// The lifecycle event deliberately contains stale preflight counters and the
				// same wall time on every call. Stamping reads the locked committed order.
				payload, err := StampLifecycle(ctx, session, 1, []byte(`{"like_count":-1,"comment_count":-1,"event_time":100,"revision":7}`))
				if err != nil {
					return err
				}
				var v struct {
					LikeCount    int64 `json:"like_count"`
					CommentCount int64 `json:"comment_count"`
					Sequence     int64 `json:"stats_seq"`
					Revision     int64 `json:"revision"`
				}
				if err = json.Unmarshal(payload, &v); err != nil {
					return err
				}
				if v.Revision != 7 {
					return errors.New("stats must not change content revision")
				}
				snapshots <- Snapshot{LikeCount: v.LikeCount, CommentCount: v.CommentCount, Sequence: v.Sequence}
				return nil
			})
			if err != nil {
				failures <- err
			}
		}(i)
	}
	wg.Wait()
	close(snapshots)
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	seen := map[int64]bool{}
	var newest Snapshot
	for snapshot := range snapshots {
		require.False(t, seen[snapshot.Sequence])
		seen[snapshot.Sequence] = true
		if snapshot.Sequence > newest.Sequence {
			newest = snapshot
		}
	}
	require.Len(t, seen, 60)
	require.Equal(t, int64(9000000000060), state.snapshot.Sequence)
	require.Equal(t, int64(20), newest.LikeCount)
	require.Equal(t, int64(20), newest.CommentCount)
}
func TestStatsAllocationRollbackAndCreateClockSkew(t *testing.T) {
	db, state := statsDB(t)
	conn := sqlstore.NewSqlConnFromDB(db)
	state.snapshot = Snapshot{}
	require.NoError(t, conn.TransactCtx(context.Background(), func(ctx context.Context, s sqlstore.Session) error {
		return SeedCreated(ctx, s, 1, []byte(`{"stats_seq":9000000000000,"event_time":100}`))
	}))
	require.Error(t, conn.TransactCtx(context.Background(), func(ctx context.Context, s sqlstore.Session) error {
		_, err := Advance(ctx, s, 1)
		if err != nil {
			return err
		}
		return errors.New("outbox failed")
	}))
	require.Equal(t, int64(9000000000000), state.snapshot.Sequence)
	require.NoError(t, conn.TransactCtx(context.Background(), func(ctx context.Context, s sqlstore.Session) error {
		v, err := Advance(ctx, s, 1)
		require.Equal(t, int64(9000000000001), v.Sequence)
		return err
	}))
}

func TestUninitializedOrExhaustedSequenceFailsClosed(t *testing.T) {
	for _, value := range []int64{0, 9223372036854775807} {
		db, state := statsDB(t)
		state.snapshot.Sequence = value
		conn := sqlstore.NewSqlConnFromDB(db)
		err := conn.TransactCtx(context.Background(), func(ctx context.Context, s sqlstore.Session) error { _, err := Advance(ctx, s, 1); return err })
		require.ErrorContains(t, err, "watermark migration")
		require.Equal(t, value, state.snapshot.Sequence)
	}
	db, _ := statsDB(t)
	conn := sqlstore.NewSqlConnFromDB(db)
	require.ErrorContains(t, conn.TransactCtx(context.Background(), func(ctx context.Context, s sqlstore.Session) error {
		return SeedCreated(ctx, s, 1, []byte(`{"stats_seq":0}`))
	}), "positive")
}
