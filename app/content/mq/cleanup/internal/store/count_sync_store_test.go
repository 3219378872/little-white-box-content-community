package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"esx/pkg/event"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

// This transactional driver exercises the production *sql.DB path, including
// rollback and lost commit acknowledgement. It is not a MySQL integration test.
type countState struct {
	mu                                   sync.Mutex
	revision, likes, favorites, statsSeq int64
	receipts                             map[int64]bool
	outbox                               int
	events                               []event.PostEvent
	fail                                 string
	loseACK                              bool
}
type countDriver struct{ s *countState }
type countConn struct {
	s                                    *countState
	revision, likes, favorites, statsSeq int64
	receipts                             map[int64]bool
	outbox                               int
	events                               []event.PostEvent
	active                               bool
}
type countRows struct {
	cols   []string
	values []driver.Value
	done   bool
}

func (r *countRows) Columns() []string { return r.cols }
func (r *countRows) Close() error      { return nil }
func (r *countRows) Next(v []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	copy(v, r.values)
	return nil
}
func (d *countDriver) Open(string) (driver.Conn, error) { return &countConn{s: d.s}, nil }
func (c *countConn) Close() error                       { return nil }
func (c *countConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *countConn) Begin() (driver.Tx, error) {
	c.s.mu.Lock()
	c.active = true
	c.events = append([]event.PostEvent(nil), c.s.events...)
	c.statsSeq = c.s.statsSeq
	c.revision, c.likes, c.favorites, c.outbox = c.s.revision, c.s.likes, c.s.favorites, c.s.outbox
	c.receipts = map[int64]bool{}
	for k, v := range c.s.receipts {
		c.receipts[k] = v
	}
	return c, nil
}
func (c *countConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) { return c.Begin() }
func (c *countConn) Rollback() error {
	if c.active {
		c.active = false
		c.s.mu.Unlock()
	}
	return nil
}
func (c *countConn) Commit() error {
	if c.s.fail == "commit" {
		c.Rollback()
		return errors.New("commit failed before durable write")
	}
	c.s.events = c.events
	c.s.statsSeq = c.statsSeq
	c.s.revision, c.s.likes, c.s.favorites, c.s.outbox, c.s.receipts = c.revision, c.likes, c.favorites, c.outbox, c.receipts
	lose := c.s.loseACK
	c.s.loseACK = false
	c.active = false
	c.s.mu.Unlock()
	if lose {
		return errors.New("commit ACK lost")
	}
	return nil
}
func (c *countConn) ExecContext(_ context.Context, q string, a []driver.NamedValue) (driver.Result, error) {
	if c.s.fail != "" && strings.Contains(q, c.s.fail) {
		return nil, errors.New("injected SQL failure")
	}
	switch {
	case strings.HasPrefix(q, "INSERT INTO content_count_receipt"):
		id := a[0].Value.(int64)
		if c.receipts[id] {
			return nil, &mysql.MySQLError{Number: 1062}
		}
		c.receipts[id] = true
	case strings.HasPrefix(q, "INSERT INTO content_count_projection"):
	case strings.HasPrefix(q, "UPDATE content_count_projection"):
		c.revision = a[0].Value.(int64)
	case strings.HasPrefix(q, "UPDATE `post` SET stats_seq"):
		if c.statsSeq <= 0 || c.statsSeq == 9223372036854775807 {
			return driver.RowsAffected(0), nil
		}
		c.statsSeq++
		return driver.RowsAffected(1), nil
	case strings.HasPrefix(q, "UPDATE `post`"):
		c.likes = a[0].Value.(int64)
		c.favorites = a[1].Value.(int64)
	case strings.HasPrefix(q, "UPDATE `comment`"):
		if strings.Contains(q, "favorite_count") {
			return nil, errors.New("comment has no favorite_count")
		}
		c.likes = a[0].Value.(int64)
	case strings.HasPrefix(q, "INSERT INTO event_outbox"):
		var e event.PostEvent
		if err := json.Unmarshal(a[4].Value.([]byte), &e); err != nil {
			return nil, err
		}
		c.events = append(c.events, e)
		c.outbox++
	default:
		return nil, fmt.Errorf("unexpected SQL %s", q)
	}
	return driver.RowsAffected(0), nil // Zero changed rows is a valid idempotent snapshot.
}
func (c *countConn) QueryContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(q, "content_count_projection_control") {
		return &countRows{cols: []string{"legacy_reconciled"}, values: []driver.Value{false}}, nil
	}
	if strings.Contains(q, "content_count_projection") {
		return &countRows{cols: []string{"revision"}, values: []driver.Value{c.revision}}, nil
	}
	if strings.Contains(q, "SELECT like_count, comment_count, stats_seq") {
		return &countRows{cols: []string{"like_count", "comment_count", "stats_seq"}, values: []driver.Value{c.likes, int64(2), c.statsSeq}}, nil
	}
	if strings.Contains(q, "`post`") {
		return &countRows{cols: []string{"comment_count"}, values: []driver.Value{int64(2)}}, nil
	}
	if strings.Contains(q, "`comment`") {
		return &countRows{cols: []string{"id"}, values: []driver.Value{int64(55)}}, nil
	}
	return nil, fmt.Errorf("unexpected query %s", q)
}

var driverID atomic.Int64

func newCountTestDB(t *testing.T) (*sql.DB, *countState) {
	t.Helper()
	s := &countState{receipts: map[int64]bool{}, statsSeq: 100}
	name := fmt.Sprintf("counts-%d", driverID.Add(1))
	sql.Register(name, &countDriver{s})
	db, e := sql.Open(name, "")
	require.NoError(t, e)
	t.Cleanup(func() { db.Close() })
	return db, s
}
func likeEvent(id, targetID int64, action, targetType string) event.BehaviorEvent {
	return event.BehaviorEvent{EventID: id, ClientEventID: "client", SchemaVersion: event.BehaviorSchemaVersion, EventTime: 100, ReceivedAt: 100, UserID: 7, Action: action, TargetID: targetID, TargetType: targetType, Producer: "business-outbox"}
}
func snapshotEvent(id, revision, likes, favorites int64) event.BehaviorEvent {
	e := likeEvent(id, 55, event.BehaviorActionLike, "post")
	e.CountSnapshot = &event.InteractionCountSnapshot{Revision: revision, LikeCount: likes, FavoriteCount: favorites}
	return e
}
func TestCountSyncCrashBeforeCommitAllowsRetry(t *testing.T) {
	for _, boundary := range []string{"INSERT INTO content_count_projection", "UPDATE `post`", "INSERT INTO event_outbox", "UPDATE content_count_projection", "commit"} {
		t.Run(boundary, func(t *testing.T) {
			db, state := newCountTestDB(t)
			s := NewCountSyncStore(db, nil)
			state.fail = boundary
			e := snapshotEvent(1, 1, 1, 1)
			require.Error(t, s.ApplyBehaviorCount(context.Background(), e))
			require.Empty(t, state.receipts)
			require.Equal(t, int64(100), state.statsSeq)
			require.Empty(t, state.events)
			require.Zero(t, state.likes)
			state.fail = ""
			require.NoError(t, s.ApplyBehaviorCount(context.Background(), e))
			require.Equal(t, int64(1), state.likes)
			require.Equal(t, 1, state.outbox)
		})
	}
}
func TestCountSyncLostACKAndRedisLossAreIdempotent(t *testing.T) {
	db, state := newCountTestDB(t)
	s := NewCountSyncStore(db, nil)
	state.loseACK = true
	e := snapshotEvent(1, 1, 1, 1)
	require.Error(t, s.ApplyBehaviorCount(context.Background(), e))
	s = NewCountSyncStore(db, nil)
	require.NoError(t, s.ApplyBehaviorCount(context.Background(), e))
	require.Equal(t, int64(1), state.favorites)
	require.Equal(t, 1, state.outbox)
}
func TestCountSyncReversedAndConcurrentTransitions(t *testing.T) {
	db, state := newCountTestDB(t)
	s := NewCountSyncStore(db, nil)
	ctx := context.Background()
	require.NoError(t, s.ApplyBehaviorCount(ctx, snapshotEvent(2, 2, 0, 0)))
	require.NoError(t, s.ApplyBehaviorCount(ctx, snapshotEvent(1, 1, 1, 1)))
	require.Zero(t, state.likes)
	require.Zero(t, state.favorites)
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := int64(3); i <= 42; i++ {
		wg.Add(1)
		go func(i int64) {
			defer wg.Done()
			e := snapshotEvent(i, i, i, i/2)
			e.UserID = i
			errs <- s.ApplyBehaviorCount(ctx, e)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, int64(42), state.revision)
	require.Equal(t, int64(42), state.likes)
	require.Equal(t, int64(21), state.favorites)
}
func TestCountSyncCommentAndLegacyValidation(t *testing.T) {
	db, state := newCountTestDB(t)
	s := NewCountSyncStore(db, nil)
	ctx := context.Background()
	legacy := likeEvent(1, 55, event.BehaviorActionLike, "post")
	require.ErrorContains(t, s.ApplyBehaviorCount(ctx, legacy), "snapshot required")
	require.Empty(t, state.receipts)
	e := snapshotEvent(2, 2, 3, 0)
	e.TargetType = "comment"
	require.NoError(t, s.ApplyBehaviorCount(ctx, e))
	require.Equal(t, int64(3), state.likes)
	require.Zero(t, state.outbox)
	legacy.Action = event.BehaviorActionClick
	require.NoError(t, s.ApplyBehaviorCount(ctx, legacy))
}

type singleSlotRedis struct{ calls [][]string }

func (r *singleSlotRedis) DelCtx(_ context.Context, keys ...string) (int, error) {
	r.calls = append(r.calls, keys)
	if len(keys) != 1 {
		return 0, errors.New("CROSSSLOT")
	}
	return 1, nil
}
func TestCountCacheInvalidationUsesSingleKeyCommands(t *testing.T) {
	db, _ := newCountTestDB(t)
	redis := &singleSlotRedis{}
	s := NewCountSyncStore(db, redis)
	require.NoError(t, s.ApplyBehaviorCount(context.Background(), snapshotEvent(1, 1, 1, 0)))
	require.Len(t, redis.calls, 2)
	for _, keys := range redis.calls {
		require.Len(t, keys, 1)
	}
}

func TestCountOutboxSnapshotCarriesCommittedSharedSequence(t *testing.T) {
	db, state := newCountTestDB(t)
	s := NewCountSyncStore(db, nil)
	require.NoError(t, s.ApplyBehaviorCount(context.Background(), snapshotEvent(1, 1, 7, 4)))
	require.Len(t, state.events, 1)
	require.Equal(t, state.statsSeq, state.events[0].StatsSeq)
	require.Equal(t, int64(101), state.events[0].StatsSeq)
	require.Equal(t, int64(7), state.events[0].LikeCount)
	require.Equal(t, int64(2), state.events[0].CommentCount)
}
