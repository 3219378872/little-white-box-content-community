package logic

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"
	"testing"

	"esx/app/pipeline/behaviorlog/internal/dedup"
	"esx/app/pipeline/behaviorlog/internal/store"
	"esx/pkg/event"

	"github.com/stretchr/testify/require"
)

type recoveryKeys struct {
	mu           sync.Mutex
	keys         map[string]string
	markError    bool
	checkBarrier chan struct{}
	checks       int
}

func (s *recoveryKeys) Exists(_ context.Context, key string) (bool, error) {
	s.mu.Lock()
	_, exists := s.keys[key]
	barrier := s.checkBarrier
	if barrier != nil {
		s.checks++
		if s.checks == 2 {
			close(barrier)
			s.checkBarrier = nil
		}
	}
	s.mu.Unlock()
	if barrier != nil {
		<-barrier
	}
	return exists, nil
}
func (s *recoveryKeys) Set(_ context.Context, key, value string, _ int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.markError {
		s.markError = false
		return errors.New("receipt write failed")
	}
	s.keys[key] = value
	return nil
}

type recoverySQL struct {
	mu                  sync.Mutex
	raw                 [][]driver.Value
	failBefore, loseACK bool
}
type recoveryDriver struct{ s *recoverySQL }

func (d recoveryDriver) Connect(context.Context) (driver.Conn, error) {
	return &recoveryConn{s: d.s}, nil
}
func (d recoveryDriver) Driver() driver.Driver            { return d }
func (d recoveryDriver) Open(string) (driver.Conn, error) { return d.Connect(context.Background()) }

type recoveryConn struct{ s *recoverySQL }

func (c *recoveryConn) Close() error { return nil }
func (c *recoveryConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *recoveryConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected transaction") }
func (c *recoveryConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	if !strings.Contains(query, "INSERT INTO xbh_analytics.behavior_events") {
		return nil, errors.New("unexpected SQL")
	}
	if c.s.failBefore {
		c.s.failBefore = false
		return nil, errors.New("insert failed")
	}
	row := make([]driver.Value, len(args))
	for i, v := range args {
		row[i] = v.Value
	}
	c.s.raw = append(c.s.raw, row)
	if c.s.loseACK {
		c.s.loseACK = false
		return nil, errors.New("insert durable but ACK lost")
	}
	return driver.RowsAffected(1), nil
}
func exposureFixture() event.BehaviorEvent {
	e := validBehaviorEvent()
	e.Action = "exposure"
	e.RequestID = "request:with:separators"
	e.Scene = "home"
	e.Position = new(int32(1))
	return e
}
func sqlRecorder(t *testing.T, state *recoverySQL, keys *recoveryKeys) *Recorder {
	t.Helper()
	db := sql.OpenDB(recoveryDriver{s: state})
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return NewRecorder(store.NewClickHouseStore(db), dedup.NewExactDedup(keys, 7776000))
}
func TestExposureBusinessKeyKeepsEnvelopeAndSurvivesRestart(t *testing.T) {
	keys := &recoveryKeys{keys: map[string]string{}}
	state := &recoverySQL{}
	first := exposureFixture()
	second := first
	second.EventID = 2
	second.ClientEventID = "different-event"
	require.NoError(t, sqlRecorder(t, state, keys).Process(context.Background(), first, MessageMeta{}))
	require.NoError(t, sqlRecorder(t, state, keys).Process(context.Background(), second, MessageMeta{}))
	require.Len(t, state.raw, 1)
	require.Equal(t, first.EventID, state.raw[0][0])
	require.Equal(t, first.ClientEventID, state.raw[0][1])
	require.Len(t, keys.keys, 1)
	require.Equal(t, behaviorDedupKey(first), behaviorDedupKey(second))
}
func TestExposureSQLFailuresNeverMarkBeforeDurableInsert(t *testing.T) {
	for _, fault := range []string{"before_insert", "lost_ack", "mark_error"} {
		t.Run(fault, func(t *testing.T) {
			keys := &recoveryKeys{keys: map[string]string{}, markError: fault == "mark_error"}
			state := &recoverySQL{failBefore: fault == "before_insert", loseACK: fault == "lost_ack"}
			e := exposureFixture()
			require.Error(t, sqlRecorder(t, state, keys).Process(context.Background(), e, MessageMeta{}))
			require.Empty(t, keys.keys)
			// A fresh recorder retries unacknowledged effects; raw envelopes may repeat.
			// The canonical SQL view, covered by integration tests, is the effect boundary.
			require.NoError(t, sqlRecorder(t, state, keys).Process(context.Background(), e, MessageMeta{}))
			require.Len(t, keys.keys, 1)
			want := 2
			if fault == "before_insert" {
				want = 1
			}
			require.Len(t, state.raw, want)
			require.NoError(t, sqlRecorder(t, state, keys).Process(context.Background(), e, MessageMeta{}))
			require.Len(t, state.raw, want)
		})
	}
}
func TestConcurrentExposureDeliveryAndReceiptLossRemainRecoverable(t *testing.T) {
	keys := &recoveryKeys{keys: map[string]string{}, checkBarrier: make(chan struct{})}
	state := &recoverySQL{}
	first := exposureFixture()
	second := first
	second.EventID = 2
	second.ClientEventID = "parallel"
	recorder := sqlRecorder(t, state, keys)
	errs := make(chan error, 2)
	go func() { errs <- recorder.Process(context.Background(), first, MessageMeta{}) }()
	go func() { errs <- recorder.Process(context.Background(), second, MessageMeta{}) }()
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	require.Len(t, state.raw, 2)
	require.Len(t, keys.keys, 1)
	freshKeys := &recoveryKeys{keys: map[string]string{}}
	require.NoError(t, sqlRecorder(t, state, freshKeys).Process(context.Background(), first, MessageMeta{}))
	require.Len(t, state.raw, 3)
	require.Equal(t, behaviorDedupKey(first), behaviorDedupKey(second))
}
func TestExposureIdentityDoesNotMergeOtherBusinessEvents(t *testing.T) {
	first := exposureFixture()
	other := first
	other.RequestID += "-other"
	require.NotEqual(t, behaviorDedupKey(first), behaviorDedupKey(other))
	other = first
	other.TargetID++
	require.NotEqual(t, behaviorDedupKey(first), behaviorDedupKey(other))
	other = first
	other.Action = "click"
	require.Equal(t, other.EventIDString(), behaviorDedupKey(other))
	require.NotEqual(t, behaviorDedupKey(first), behaviorDedupKey(other))
	other.EventID++
	require.Equal(t, other.EventIDString(), behaviorDedupKey(other))
}
