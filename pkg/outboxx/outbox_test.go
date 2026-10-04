package outboxx

import (
	"context"
	"database/sql"
	"testing"

	sqlx "esx/pkg/sqlstore"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// execSession is a non-transactional session: its writes are visible on return.
type execSession struct {
	sqlx.Session
	execs int
}

func (s *execSession) ExecCtx(context.Context, string, ...any) (sql.Result, error) {
	s.execs++
	return nil, nil
}

func TestSQLStoreEnqueueOutsideManagedTransactionWakesRelayOnce(t *testing.T) {
	store := NewSQLStore(nil)
	session := &execSession{}

	require.NoError(t, store.Enqueue(context.Background(), session, Event{ID: 1, Topic: "t", Key: "k", Payload: []byte("{}")}))
	require.NoError(t, store.Enqueue(context.Background(), session, Event{ID: 2, Topic: "t", Key: "k", Payload: []byte("{}")}))

	assert.Equal(t, 2, session.execs)
	select {
	case <-store.Wakeups():
	default:
		t.Fatal("enqueue outside a managed transaction must wake the relay")
	}
	select {
	case <-store.Wakeups():
		t.Fatal("wakeups must coalesce instead of queueing one per event")
	default:
	}
}

func TestSQLStoreEnqueueRejectsInvalidEventWithoutWakeup(t *testing.T) {
	store := NewSQLStore(nil)
	session := &execSession{}

	assert.Error(t, store.Enqueue(context.Background(), session, Event{ID: 1}))

	assert.Zero(t, session.execs)
	select {
	case <-store.Wakeups():
		t.Fatal("invalid event must not wake the relay")
	default:
	}
}
