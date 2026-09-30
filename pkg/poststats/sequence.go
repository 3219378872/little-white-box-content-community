// Package poststats owns the shared transactional ordering of post-count snapshots.
package poststats

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"esx/pkg/sqlstore"
)

// Snapshot is read while the post row is locked by its mutation transaction.
type Snapshot struct {
	LikeCount    int64 `db:"like_count"`
	CommentCount int64 `db:"comment_count"`
	Sequence     int64 `db:"stats_seq"`
}

const advanceSQL = "UPDATE `post` SET stats_seq = stats_seq + 1 WHERE id = ? AND stats_seq > 0 AND stats_seq < 9223372036854775807"
const snapshotSQL = "SELECT like_count, comment_count, stats_seq FROM `post` WHERE id = ?"

// Advance must use the same transaction as the count mutation and outbox write.
// UPDATE holds the post row lock, so comment and interaction snapshots share one
// durable order regardless of process clocks or event delivery order.
func Advance(ctx context.Context, session sqlstore.Session, postID int64) (Snapshot, error) {
	result, err := session.ExecCtx(ctx, advanceSQL, postID)
	if err = expectPost(result, err); err != nil {
		return Snapshot{}, err
	}
	var snapshot Snapshot
	err = session.QueryRowCtx(ctx, &snapshot, snapshotSQL, postID)
	return snapshot, err
}

// AdvanceTx adapts the same sequence boundary to database/sql transactions.
func AdvanceTx(ctx context.Context, tx *sql.Tx, postID int64) (Snapshot, error) {
	result, err := tx.ExecContext(ctx, advanceSQL, postID)
	if err = expectPost(result, err); err != nil {
		return Snapshot{}, err
	}
	var snapshot Snapshot
	err = tx.QueryRowContext(ctx, snapshotSQL, postID).Scan(&snapshot.LikeCount, &snapshot.CommentCount, &snapshot.Sequence)
	return snapshot, err
}
func expectPost(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("post stats sequence is missing, uninitialized, or exhausted; complete watermark migration before writes")
	}
	return nil
}

// SeedCreated starts a new post above its create-event watermark. Historical
// rows instead require the explicit paused-writer migration; wall time cannot
// establish a bound on a previous producer's clock.
func SeedCreated(ctx context.Context, session sqlstore.Session, postID int64, payload []byte) error {
	var watermark struct {
		Sequence  int64 `json:"stats_seq"`
		EventTime int64 `json:"event_time"`
	}
	if err := json.Unmarshal(payload, &watermark); err != nil {
		return fmt.Errorf("decode create stats watermark: %w", err)
	}
	if watermark.Sequence <= 0 {
		watermark.Sequence = watermark.EventTime
	}
	if watermark.Sequence <= 0 {
		return fmt.Errorf("create stats watermark must be positive")
	}
	_, err := session.ExecCtx(ctx, "UPDATE `post` SET stats_seq = GREATEST(stats_seq, ?) WHERE id = ?", watermark.Sequence, postID)
	return err
}

// StampLifecycle overwrites preflight counters with the locked transaction's
// snapshot and advances only the independent stats sequence, not content revision.
func StampLifecycle(ctx context.Context, session sqlstore.Session, postID int64, payload []byte) ([]byte, error) {
	snapshot, err := Advance(ctx, session, postID)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(payload, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("post lifecycle event must be an object")
	}
	for name, value := range map[string]int64{"like_count": snapshot.LikeCount, "comment_count": snapshot.CommentCount, "stats_seq": snapshot.Sequence} {
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		fields[name] = encoded
	}
	return json.Marshal(fields)
}
