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

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
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
