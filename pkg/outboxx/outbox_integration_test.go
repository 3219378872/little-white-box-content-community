//go:build integration

package outboxx

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	sqlx "esx/pkg/sqlstore"

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
)

func setupOutboxMySQL(t *testing.T) (*sql.DB, *SQLStore) {
	t.Helper()
	ctx := context.Background()
	container, err := mysql.Run(ctx,
		"mysql:8.0",
		mysql.WithDatabase("outbox_test"),
		mysql.WithUsername("root"),
		mysql.WithPassword("testpass"),
		mysql.WithScripts(filepath.Join("testdata", "schema.sql")),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })

	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	require.NoError(t, err)
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	require.NoError(t, db.PingContext(ctx))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db, NewSQLStore(sqlx.NewMysql(dsn))
}

func outboxEvent(id int64) Event {
	return Event{ID: id, Topic: "user-behavior-v2", Tag: "default", Key: "event-key", Payload: []byte(`{"event_id":1}`)}
}

func TestSQLStoreTransactionRollbackLeaseRecoveryAndDuplicateDelivery(t *testing.T) {
	db, store := setupOutboxMySQL(t)
	ctx := context.Background()

	t.Run("transaction rollback removes event", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, store.EnqueueTx(ctx, tx, outboxEvent(1)))
		require.NoError(t, tx.Rollback())

		var count int
		require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM event_outbox WHERE id = 1").Scan(&count))
		assert.Zero(t, count)
	})

	t.Run("expired lease can be recovered", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, store.EnqueueTx(ctx, tx, outboxEvent(2)))
		require.NoError(t, tx.Commit())

		now := time.Now().Add(time.Second)
		first, err := store.Claim(ctx, "relay-a", 1, now, time.Second)
		require.NoError(t, err)
		require.Len(t, first, 1)
		assert.Equal(t, 1, first[0].Attempts)

		beforeExpiry, err := store.Claim(ctx, "relay-b", 1, now.Add(500*time.Millisecond), time.Second)
		require.NoError(t, err)
		assert.Empty(t, beforeExpiry)

		afterExpiry, err := store.Claim(ctx, "relay-b", 1, now.Add(2*time.Second), time.Second)
		require.NoError(t, err)
		require.Len(t, afterExpiry, 1)
		assert.Equal(t, 2, afterExpiry[0].Attempts)
		require.NoError(t, store.MarkSent(ctx, 2, "relay-b", now.Add(2*time.Second)))
	})

	t.Run("broker ack before crash is delivered again after lease", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, store.EnqueueTx(ctx, tx, outboxEvent(3)))
		require.NoError(t, tx.Commit())

		base := time.Now().Add(5 * time.Second)
		var publishes atomic.Int32
		firstRelay, err := NewRelay(store, PublisherFunc(func(context.Context, Record) error {
			publishes.Add(1)
			return nil
		}), RelayConfig{
			Owner: "crashing-relay", BatchSize: 1, PollInterval: time.Second,
			Lease: time.Second, BaseBackoff: time.Second, MaxBackoff: time.Minute, MaxAttempts: 3,
		})
		require.NoError(t, err)
		firstRelay.now = func() time.Time { return base }

		records, err := store.Claim(ctx, firstRelay.config.Owner, 1, base, time.Second)
		require.NoError(t, err)
		require.Len(t, records, 1)
		require.NoError(t, firstRelay.publisher.Publish(ctx, records[0]))
		// Deliberately skip MarkSent to model a process crash after broker ACK.

		secondRelay, err := NewRelay(store, PublisherFunc(func(context.Context, Record) error {
			publishes.Add(1)
			return nil
		}), RelayConfig{
			Owner: "recovery-relay", BatchSize: 1, PollInterval: time.Second,
			Lease: time.Second, BaseBackoff: time.Second, MaxBackoff: time.Minute, MaxAttempts: 3,
		})
		require.NoError(t, err)
		secondRelay.now = func() time.Time { return base.Add(2 * time.Second) }
		processed, err := secondRelay.ProcessBatch(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, processed)
		assert.Equal(t, int32(2), publishes.Load())

		backlog, err := store.Backlog(ctx)
		require.NoError(t, err)
		assert.Zero(t, backlog.Count)
	})
}

func TestRelayDrainsOldBacklogAcrossMultipleBatches(t *testing.T) {
	db, store := setupOutboxMySQL(t)
	ctx := context.Background()
	const eventCount = 205
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	for index := int64(1); index <= eventCount; index++ {
		event := outboxEvent(10_000 + index)
		event.Key = "backlog-" + fmt.Sprint(index)
		require.NoError(t, store.EnqueueTx(ctx, tx, event))
	}
	require.NoError(t, tx.Commit())
	oldest := time.Now().Add(-2 * time.Hour).Truncate(time.Millisecond)
	_, err = db.ExecContext(ctx, "UPDATE event_outbox SET created_at = ?", oldest.UnixMilli())
	require.NoError(t, err)

	backlog, err := store.Backlog(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(eventCount), backlog.Count)
	assert.Equal(t, oldest.UnixMilli(), backlog.OldestCreatedAt)

	var published atomic.Int32
	relay, err := NewRelay(store, PublisherFunc(func(context.Context, Record) error {
		published.Add(1)
		return nil
	}), RelayConfig{
		Owner: "backlog-relay", BatchSize: 50, PollInterval: time.Millisecond,
		Lease: time.Second, BaseBackoff: time.Millisecond, MaxBackoff: time.Second, MaxAttempts: 3,
	})
	require.NoError(t, err)
	for {
		processed, processErr := relay.ProcessBatch(ctx)
		require.NoError(t, processErr)
		if processed == 0 {
			break
		}
	}

	assert.Equal(t, int32(eventCount), published.Load())
	backlog, err = store.Backlog(ctx)
	require.NoError(t, err)
	assert.Zero(t, backlog.Count)
	assert.Zero(t, backlog.OldestCreatedAt)
}

func enqueueCommitted(t *testing.T, db *sql.DB, store *SQLStore, ids ...int64) {
	t.Helper()
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	for _, id := range ids {
		require.NoError(t, store.EnqueueTx(context.Background(), tx, outboxEvent(id)))
	}
	require.NoError(t, tx.Commit())
}

func recordIDs(records []Record) []int64 {
	ids := make([]int64, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	return ids
}

func drainWakeups(store *SQLStore) {
	select {
	case <-store.Wakeups():
	default:
	}
}

func TestSQLStoreEnqueueWakesRelayOnlyAfterCommit(t *testing.T) {
	_, store := setupOutboxMySQL(t)
	ctx := context.Background()
	drainWakeups(store)

	err := store.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		if err := store.Enqueue(ctx, session, outboxEvent(1)); err != nil {
			return err
		}
		select {
		case <-store.Wakeups():
			t.Error("relay woken before the enqueuing transaction committed")
		default:
		}
		return nil
	})
	require.NoError(t, err)
	select {
	case <-store.Wakeups():
	default:
		t.Fatal("committed enqueue must wake the relay")
	}

	rollback := fmt.Errorf("business failure")
	err = store.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		if err := store.Enqueue(ctx, session, outboxEvent(2)); err != nil {
			return err
		}
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	select {
	case <-store.Wakeups():
		t.Fatal("rolled back enqueue must not wake the relay")
	default:
	}
}

func TestSQLStoreClaimSkipsLockedRowsWithoutBlocking(t *testing.T) {
	db, store := setupOutboxMySQL(t)
	ctx := context.Background()
	enqueueCommitted(t, db, store, 1, 2, 3)

	// Another relay is mid-claim on row 1.
	holder, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Rollback() })
	var locked int64
	require.NoError(t, holder.QueryRowContext(ctx, "SELECT id FROM event_outbox WHERE id = 1 FOR UPDATE").Scan(&locked))

	// A business transaction has enqueued row 4 but not committed yet.
	business, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = business.Rollback() })
	require.NoError(t, store.EnqueueTx(ctx, business, outboxEvent(4)))

	claimCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	claimed, err := store.Claim(claimCtx, "relay-b", 10, time.Now().Add(time.Second), time.Minute)
	require.NoError(t, err, "claim must skip locked rows instead of waiting on them")
	assert.Equal(t, []int64{2, 3}, recordIDs(claimed))

	require.NoError(t, holder.Rollback())
	require.NoError(t, business.Commit())
	claimed, err = store.Claim(ctx, "relay-b", 10, time.Now().Add(time.Second), time.Minute)
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 4}, recordIDs(claimed))
}

func TestSQLStoreClaimDoesNotBlockConcurrentEnqueue(t *testing.T) {
	db, store := setupOutboxMySQL(t)
	ctx := context.Background()
	enqueueCommitted(t, db, store, 1)

	// Reproduce the claim's locking reads inside an open transaction, then
	// require a business insert into the same index range to proceed.
	claimer, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	require.NoError(t, err)
	t.Cleanup(func() { _ = claimer.Rollback() })
	rows, err := selectClaimable(ctx, claimer, claimDueSQL, StatusPending, time.Now().Add(time.Hour).UnixMilli(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	insertCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := db.BeginTx(insertCtx, nil)
	require.NoError(t, err)
	require.NoError(t, store.EnqueueTx(insertCtx, tx, outboxEvent(2)), "claim locks must not block business inserts")
	require.NoError(t, tx.Commit())
}

func TestSQLStoreClaimServesExpiredLeasesAndRetriesBeforeFreshEvents(t *testing.T) {
	db, store := setupOutboxMySQL(t)
	ctx := context.Background()
	enqueueCommitted(t, db, store, 1, 2, 3, 4)
	now := time.Now().Add(time.Second)
	nowMillis := now.UnixMilli()
	_, err := db.ExecContext(ctx, `UPDATE event_outbox SET status = ?, locked_by = 'dead', locked_until = ? WHERE id = 3`,
		StatusProcessing, nowMillis-1)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE event_outbox SET status = ?, next_attempt_at = ? WHERE id = 4`,
		StatusRetry, nowMillis-1)
	require.NoError(t, err)

	claimed, err := store.Claim(ctx, "relay-a", 3, now, time.Minute)
	require.NoError(t, err)

	assert.Equal(t, []int64{3, 4, 1}, recordIDs(claimed))
	for _, record := range claimed {
		assert.Equal(t, "relay-a", record.LockedBy)
	}
	var leased int
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT count(*) FROM event_outbox WHERE status = ? AND locked_by = 'relay-a' AND locked_until = ?",
		StatusProcessing, now.Add(time.Minute).UnixMilli()).Scan(&leased))
	assert.Equal(t, 3, leased)
}

func TestSQLStorePurgeDeletesOnlyOldSentEvents(t *testing.T) {
	db, store := setupOutboxMySQL(t)
	ctx := context.Background()
	enqueueCommitted(t, db, store, 1, 2, 3, 4, 5, 6)
	cutoff := time.Now().Add(-24 * time.Hour)
	old, recent := cutoff.Add(-time.Hour).UnixMilli(), cutoff.Add(time.Hour).UnixMilli()
	for id, row := range map[int64]struct {
		status    int8
		createdAt int64
	}{
		1: {StatusSent, old}, 2: {StatusSent, old}, 3: {StatusSent, recent},
		4: {StatusDead, old}, 5: {StatusRetry, old}, 6: {StatusPending, old},
	} {
		_, err := db.ExecContext(ctx, "UPDATE event_outbox SET status = ?, created_at = ? WHERE id = ?", row.status, row.createdAt, id)
		require.NoError(t, err)
	}

	first, err := store.Purge(ctx, cutoff, 1)
	require.NoError(t, err)
	second, err := store.Purge(ctx, cutoff, 10)
	require.NoError(t, err)
	third, err := store.Purge(ctx, cutoff, 10)
	require.NoError(t, err)

	assert.Equal(t, []int64{1, 1, 0}, []int64{first, second, third})
	rows, err := db.QueryContext(ctx, "SELECT id FROM event_outbox ORDER BY id")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var remaining []int64
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		remaining = append(remaining, id)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []int64{3, 4, 5, 6}, remaining)
}
