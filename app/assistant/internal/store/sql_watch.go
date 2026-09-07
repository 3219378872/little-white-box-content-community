package store

import (
	"context"
	"fmt"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"strconv"
)

func (s *SQLStore) UpsertDeliveryBucket(ctx context.Context, userID, hitID, windowStartMs, nowMs int64) (DeliveryBucket, error) {
	var existing DeliveryBucket
	err := s.exec.QueryRowCtx(ctx, &struct {
		ID            int64  `db:"id"`
		UserID        int64  `db:"user_id"`
		WindowStartMs int64  `db:"window_start_ms"`
		Status        string `db:"status"`
		HitIDs        []byte `db:"hit_ids"`
		RunID         int64  `db:"run_id"`
		CreatedAtMs   int64  `db:"created_at_ms"`
	}{}, `SELECT id FROM watch_delivery_bucket WHERE user_id=? AND window_start_ms=?`, userID, windowStartMs)
	_ = err
	res, err := s.exec.ExecCtx(ctx, `INSERT INTO watch_delivery_bucket (user_id, window_start_ms, status, hit_ids, run_id, created_at_ms)
		VALUES (?, ?, 'pending', ?, 0, ?)
		ON DUPLICATE KEY UPDATE hit_ids = JSON_ARRAY_APPEND(IFNULL(hit_ids, JSON_ARRAY()), '$', ?)`,
		userID, windowStartMs, mustJSON([]int64{hitID}), nowMs, hitID)
	if err != nil {
		return DeliveryBucket{}, err
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		var row struct {
			ID            int64  `db:"id"`
			UserID        int64  `db:"user_id"`
			WindowStartMs int64  `db:"window_start_ms"`
			NotBeforeMs   int64  `db:"not_before_ms"`
			Status        string `db:"status"`
			HitIDs        []byte `db:"hit_ids"`
			RunID         int64  `db:"run_id"`
			CreatedAtMs   int64  `db:"created_at_ms"`
		}
		if err := s.exec.QueryRowCtx(ctx, &row, `SELECT id, user_id, window_start_ms, not_before_ms, status, hit_ids, run_id, created_at_ms
			FROM watch_delivery_bucket WHERE user_id=? AND window_start_ms=?`, userID, windowStartMs); err != nil {
			return DeliveryBucket{}, err
		}
		existing = DeliveryBucket{ID: row.ID, UserID: row.UserID, WindowStartMs: row.WindowStartMs, NotBeforeMs: row.NotBeforeMs, Status: row.Status,
			HitIDs: decodeInt64s(row.HitIDs), RunID: row.RunID, CreatedAtMs: row.CreatedAtMs}
		return existing, nil
	}
	return DeliveryBucket{ID: id, UserID: userID, WindowStartMs: windowStartMs, Status: "pending", HitIDs: []int64{hitID}, CreatedAtMs: nowMs}, nil
}

func (s *SQLStore) GetBucket(ctx context.Context, id int64) (*DeliveryBucket, error) {
	return s.scanBucket(ctx, `SELECT id, user_id, window_start_ms, not_before_ms, status, hit_ids, run_id, created_at_ms FROM watch_delivery_bucket WHERE id=?`, id)
}

func (s *SQLStore) GetPendingBucket(ctx context.Context, userID int64) (*DeliveryBucket, error) {
	return s.scanBucket(ctx, `SELECT id, user_id, window_start_ms, not_before_ms, status, hit_ids, run_id, created_at_ms
		FROM watch_delivery_bucket WHERE user_id=? AND status IN ('pending','deferred') ORDER BY window_start_ms ASC LIMIT 1`, userID)
}

func (s *SQLStore) ListDueBuckets(ctx context.Context, nowMs, windowMs int64) ([]DeliveryBucket, error) {
	var rows []struct {
		ID            int64  `db:"id"`
		UserID        int64  `db:"user_id"`
		WindowStartMs int64  `db:"window_start_ms"`
		NotBeforeMs   int64  `db:"not_before_ms"`
		Status        string `db:"status"`
		HitIDs        []byte `db:"hit_ids"`
		RunID         int64  `db:"run_id"`
		CreatedAtMs   int64  `db:"created_at_ms"`
	}
	if err := s.exec.QueryRowsCtx(ctx, &rows, `SELECT id, user_id, window_start_ms, not_before_ms, status, hit_ids, run_id, created_at_ms
		FROM watch_delivery_bucket WHERE status IN ('pending','deferred')
		AND ((not_before_ms=0 AND window_start_ms + ? <= ?) OR (not_before_ms>0 AND not_before_ms <= ?))
		ORDER BY window_start_ms ASC LIMIT 50`, windowMs, nowMs, nowMs); err != nil {
		return nil, err
	}
	out := make([]DeliveryBucket, 0, len(rows))
	for _, row := range rows {
		out = append(out, DeliveryBucket{ID: row.ID, UserID: row.UserID, WindowStartMs: row.WindowStartMs, NotBeforeMs: row.NotBeforeMs, Status: row.Status,
			HitIDs: decodeInt64s(row.HitIDs), RunID: row.RunID, CreatedAtMs: row.CreatedAtMs})
	}
	return out, nil
}

func (s *SQLStore) scanBucket(ctx context.Context, query string, args ...any) (*DeliveryBucket, error) {
	var row struct {
		ID            int64  `db:"id"`
		UserID        int64  `db:"user_id"`
		WindowStartMs int64  `db:"window_start_ms"`
		NotBeforeMs   int64  `db:"not_before_ms"`
		Status        string `db:"status"`
		HitIDs        []byte `db:"hit_ids"`
		RunID         int64  `db:"run_id"`
		CreatedAtMs   int64  `db:"created_at_ms"`
	}
	if err := s.exec.QueryRowCtx(ctx, &row, query, args...); err != nil {
		if err == sqlx.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &DeliveryBucket{ID: row.ID, UserID: row.UserID, WindowStartMs: row.WindowStartMs, NotBeforeMs: row.NotBeforeMs, Status: row.Status,
		HitIDs: decodeInt64s(row.HitIDs), RunID: row.RunID, CreatedAtMs: row.CreatedAtMs}, nil
}

func (s *SQLStore) MarkBucketScheduled(ctx context.Context, id, runID int64) error {
	res, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='scheduled', run_id=?, not_before_ms=0
		WHERE id=? AND status IN ('pending','deferred') AND run_id=0`, runID, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("watch bucket %d schedule CAS failed", id)
	}
	return nil
}

func (s *SQLStore) MarkBucketSent(ctx context.Context, id, runID int64) error {
	res, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='sent' WHERE id=? AND run_id=? AND status='scheduled'`, id, runID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("watch bucket %d is not scheduled for run %d", id, runID)
	}
	return nil
}

func (s *SQLStore) DeferBucket(ctx context.Context, id, notBeforeMs int64) error {
	_, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='deferred', run_id=0, not_before_ms=? WHERE id=? AND status IN ('pending','deferred')`, notBeforeMs, id)
	return err
}

func (s *SQLStore) DismissBucket(ctx context.Context, id, runID int64) error {
	if s.conn != nil {
		return s.Transact(ctx, func(ctx context.Context, tx Store) error { return tx.DismissBucket(ctx, id, runID) })
	}
	query := `UPDATE watch_delivery_bucket SET status='discarded', run_id=0, not_before_ms=0
		WHERE id=? AND run_id=0 AND status IN ('pending','deferred')`
	args := []any{id}
	if runID > 0 {
		query = `UPDATE watch_delivery_bucket SET status='discarded', run_id=0, not_before_ms=0
			WHERE id=? AND run_id=? AND status='scheduled'`
		args = append(args, runID)
	}
	res, err := s.exec.ExecCtx(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil || affected == 0 {
		return err
	}
	return s.releaseWatchQuota(ctx, id, false)
}

func (s *SQLStore) ResetBucket(ctx context.Context, id, runID int64) error {
	if s.conn != nil {
		return s.Transact(ctx, func(ctx context.Context, tx Store) error { return tx.ResetBucket(ctx, id, runID) })
	}
	res, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='pending', run_id=0, not_before_ms=0 WHERE id=? AND run_id=? AND status='scheduled'`, id, runID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil || affected == 0 {
		return err
	}
	return s.releaseWatchQuota(ctx, id, false)
}

func (s *SQLStore) RequeueFailedBuckets(ctx context.Context, nowMs int64) error {
	if s.conn != nil {
		return s.Transact(ctx, func(ctx context.Context, tx Store) error { return tx.RequeueFailedBuckets(ctx, nowMs) })
	}
	var candidates []struct {
		BucketID int64 `db:"bucket_id"`
		UserID   int64 `db:"user_id"`
		RunID    int64 `db:"run_id"`
	}
	if err := s.exec.QueryRowsCtx(ctx, &candidates, `SELECT b.id AS bucket_id, b.user_id, b.run_id FROM watch_delivery_bucket b
		JOIN agent_run r ON r.id=b.run_id AND r.user_id=b.user_id AND r.source='watch'
		WHERE b.status='scheduled' AND r.status IN ('error','cancelled')
		ORDER BY b.run_id, b.id`); err != nil {
		return err
	}
	for _, candidate := range candidates {
		var run struct {
			Status        string `db:"status"`
			QueuedPayload []byte `db:"queued_payload"`
		}
		if err := s.exec.QueryRowCtx(ctx, &run, `SELECT status, queued_payload FROM agent_run WHERE id=? FOR UPDATE`, candidate.RunID); err != nil {
			if err == sqlx.ErrNotFound {
				continue
			}
			return err
		}
		if run.Status != StatusError && run.Status != StatusCancelled {
			continue
		}
		bucket, err := s.scanBucket(ctx, `SELECT id, user_id, window_start_ms, not_before_ms, status, hit_ids, run_id, created_at_ms
			FROM watch_delivery_bucket WHERE id=? AND run_id=? FOR UPDATE`, candidate.BucketID, candidate.RunID)
		if err != nil {
			return err
		}
		if bucket == nil || bucket.Status != "scheduled" {
			continue
		}
		target, notBeforeMs := "discarded", int64(0)
		if watchBucketIDFromPayload(run.QueuedPayload) == candidate.BucketID {
			failedAttempts := 0
			if run.Status == StatusError {
				failedAttempts, err = s.countFailedWatchAttempts(ctx, candidate.BucketID, candidate.UserID, candidate.RunID, bucket.CreatedAtMs)
				if err != nil {
					return err
				}
			}
			target, notBeforeMs, err = watchTerminalBucketState(run.Status, *bucket, failedAttempts, nowMs)
			if err != nil {
				return err
			}
		}
		if err := s.releaseWatchQuota(ctx, candidate.BucketID, false); err != nil {
			return err
		}
		if _, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status=?, run_id=0, not_before_ms=?
			WHERE id=? AND run_id=? AND status='scheduled'`, target, notBeforeMs, candidate.BucketID, candidate.RunID); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLStore) FinishWatchDelivery(ctx context.Context, id, userID, runID int64, runStatus string, nowMs int64) error {
	if s.conn != nil {
		return s.Transact(ctx, func(ctx context.Context, tx Store) error {
			return tx.FinishWatchDelivery(ctx, id, userID, runID, runStatus, nowMs)
		})
	}
	bucket, err := s.scanBucket(ctx, `SELECT id, user_id, window_start_ms, not_before_ms, status, hit_ids, run_id, created_at_ms
		FROM watch_delivery_bucket WHERE id=? AND user_id=? FOR UPDATE`, id, userID)
	if err != nil {
		return err
	}
	if bucket == nil {
		return nil
	}
	if bucket.Status == "sent" {
		return nil
	}
	if bucket.Status == "pending" || bucket.Status == "deferred" || bucket.Status == "discarded" {
		return s.releaseWatchQuota(ctx, id, false)
	}
	if bucket.RunID != runID {
		// A stale finalizer may race a new delivery for the same bucket. It
		// must not fail the old run or release the new run's reservation.
		return nil
	}
	if bucket.Status != "scheduled" {
		return fmt.Errorf("watch bucket %d is not scheduled", id)
	}
	if runStatus != StatusDone {
		failedAttempts := 0
		if runStatus == StatusError {
			failedAttempts, err = s.countFailedWatchAttempts(ctx, id, userID, runID, bucket.CreatedAtMs)
			if err != nil {
				return err
			}
		}
		target, notBeforeMs, err := watchTerminalBucketState(runStatus, *bucket, failedAttempts, nowMs)
		if err != nil {
			return err
		}
		if err := s.releaseWatchQuota(ctx, id, false); err != nil {
			return err
		}
		_, err = s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status=?, run_id=0, not_before_ms=?
			WHERE id=? AND user_id=? AND run_id=? AND status='scheduled'`, target, notBeforeMs, id, userID, runID)
		return err
	}
	res, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='sent'
		WHERE id=? AND user_id=? AND run_id=? AND status='scheduled'`, id, userID, runID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("watch bucket %d delivery CAS failed", id)
	}
	var reservationCount int64
	if err := s.exec.QueryRowCtx(ctx, &reservationCount, `SELECT COUNT(*) FROM watch_send_reservation WHERE bucket_id=?`, id); err != nil {
		return err
	}
	if reservationCount > 0 {
		return s.releaseWatchQuota(ctx, id, true)
	}
	return s.commitLegacyWatchQuota(ctx, bucket, nowMs)
}

func (s *SQLStore) countFailedWatchAttempts(ctx context.Context, bucketID, userID, currentRunID, bucketCreatedAtMs int64) (int, error) {
	var current struct {
		QueuedPayload []byte `db:"queued_payload"`
	}
	if err := s.exec.QueryRowCtx(ctx, &current, `SELECT queued_payload FROM agent_run
		WHERE id=? AND user_id=? AND status=? AND source=?`, currentRunID, userID, StatusError, SourceWatch); err != nil {
		if err == sqlx.ErrNotFound {
			return watchDeliveryMaxAttempts, nil
		}
		return 0, err
	}
	if watchBucketIDFromPayload(current.QueuedPayload) != bucketID {
		return watchDeliveryMaxAttempts, nil
	}
	var historical int
	err := s.exec.QueryRowCtx(ctx, &historical, `SELECT COUNT(*) FROM (
		SELECT id FROM agent_run FORCE INDEX (idx_run_user_status)
		WHERE user_id=? AND status=? AND source=? AND id<? AND created_at_ms>=?
		AND JSON_UNQUOTE(JSON_EXTRACT(queued_payload, '$.bucket_id'))=?
		ORDER BY id DESC LIMIT ?
	) AS failed_watch_attempts`, userID, StatusError, SourceWatch, currentRunID, bucketCreatedAtMs,
		strconv.FormatInt(bucketID, 10), watchDeliveryMaxAttempts-1)
	return historical + 1, err
}

func (s *SQLStore) ResetUnsentBuckets(ctx context.Context, userID int64) error {
	if s.conn != nil {
		return s.Transact(ctx, func(ctx context.Context, tx Store) error { return tx.ResetUnsentBuckets(ctx, userID) })
	}
	var bucketIDs []int64
	if err := s.exec.QueryRowsCtx(ctx, &bucketIDs, `SELECT id FROM watch_delivery_bucket
		WHERE user_id=? AND status='scheduled' ORDER BY id FOR UPDATE`, userID); err != nil {
		return err
	}
	for _, bucketID := range bucketIDs {
		if err := s.releaseWatchQuota(ctx, bucketID, false); err != nil {
			return err
		}
		if _, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='pending', run_id=0, not_before_ms=0
			WHERE id=? AND status='scheduled'`, bucketID); err != nil {
			return err
		}
	}
	return nil
}
