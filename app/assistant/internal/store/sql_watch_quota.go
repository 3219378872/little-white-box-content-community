package store

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

func (s *SQLStore) ReserveWatchQuota(ctx context.Context, bucketID, userID int64, taskIDs []int64, dayStartMs, hourStartMs int64, dailyLimit, hourlyLimit int) (bool, int64, error) {
	if s.conn != nil {
		var allowed bool
		var retryAtMs int64
		err := s.Transact(ctx, func(ctx context.Context, tx Store) error {
			var err error
			allowed, retryAtMs, err = tx.ReserveWatchQuota(ctx, bucketID, userID, taskIDs, dayStartMs, hourStartMs, dailyLimit, hourlyLimit)
			return err
		})
		return allowed, retryAtMs, err
	}
	if bucketID <= 0 || userID <= 0 || dailyLimit <= 0 || hourlyLimit <= 0 {
		return false, 0, fmt.Errorf("invalid watch quota reservation")
	}
	var bucketRow struct {
		Status string `db:"status"`
	}
	if err := s.exec.QueryRowCtx(ctx, &bucketRow, `SELECT status FROM watch_delivery_bucket
		WHERE id=? AND user_id=? FOR UPDATE`, bucketID, userID); err != nil {
		if err == sqlx.ErrNotFound {
			return false, 0, nil
		}
		return false, 0, err
	}
	if bucketRow.Status != "pending" && bucketRow.Status != "deferred" {
		return false, 0, nil
	}
	var existing int64
	if err := s.exec.QueryRowCtx(ctx, &existing, `SELECT COUNT(*) FROM watch_send_reservation WHERE bucket_id=?`, bucketID); err != nil {
		return false, 0, err
	}
	if existing > 0 {
		return true, 0, nil
	}
	sort.Slice(taskIDs, func(i, j int) bool { return taskIDs[i] < taskIDs[j] })
	uniqueTasks := taskIDs[:0]
	for _, taskID := range taskIDs {
		if taskID <= 0 || (len(uniqueTasks) > 0 && uniqueTasks[len(uniqueTasks)-1] == taskID) {
			continue
		}
		uniqueTasks = append(uniqueTasks, taskID)
	}
	type quotaRow struct {
		taskID int64
		kind   string
		start  int64
		limit  int
		retry  int64
	}
	quotas := []quotaRow{{taskID: 0, kind: "day", start: dayStartMs, limit: dailyLimit, retry: dayStartMs + int64((24 * time.Hour).Milliseconds())}}
	for _, taskID := range uniqueTasks {
		quotas = append(quotas, quotaRow{taskID: taskID, kind: "hour", start: hourStartMs, limit: hourlyLimit, retry: hourStartMs + int64(time.Hour.Milliseconds())})
	}
	for _, quota := range quotas {
		if _, err := s.exec.ExecCtx(ctx, `INSERT INTO watch_send_stat
			(user_id, task_id, period_kind, period_start_ms, sent_count, reserved_count) VALUES (?, ?, ?, ?, 0, 0)
			ON DUPLICATE KEY UPDATE sent_count=sent_count`,
			userID, quota.taskID, quota.kind, quota.start); err != nil {
			return false, 0, err
		}
		var row struct {
			Sent     int64 `db:"sent_count"`
			Reserved int64 `db:"reserved_count"`
		}
		if err := s.exec.QueryRowCtx(ctx, &row, `SELECT sent_count, reserved_count FROM watch_send_stat
			WHERE user_id=? AND task_id=? AND period_kind=? AND period_start_ms=? FOR UPDATE`,
			userID, quota.taskID, quota.kind, quota.start); err != nil {
			return false, 0, err
		}
		if row.Sent+row.Reserved >= int64(quota.limit) {
			return false, quota.retry, nil
		}
	}
	for _, quota := range quotas {
		if _, err := s.exec.ExecCtx(ctx, `INSERT INTO watch_send_reservation
			(bucket_id, user_id, task_id, period_kind, period_start_ms, created_at_ms) VALUES (?, ?, ?, ?, ?, ?)`,
			bucketID, userID, quota.taskID, quota.kind, quota.start, NowMs()); err != nil {
			return false, 0, err
		}
		res, err := s.exec.ExecCtx(ctx, `UPDATE watch_send_stat SET reserved_count=reserved_count+1
			WHERE user_id=? AND task_id=? AND period_kind=? AND period_start_ms=?`, userID, quota.taskID, quota.kind, quota.start)
		if err != nil {
			return false, 0, err
		}
		affected, err := res.RowsAffected()
		if err != nil || affected != 1 {
			if err != nil {
				return false, 0, err
			}
			return false, 0, fmt.Errorf("watch quota reservation stat missing")
		}
	}
	return true, 0, nil
}

func (s *SQLStore) releaseWatchQuota(ctx context.Context, bucketID int64, delivered bool) error {
	var rows []struct {
		UserID        int64  `db:"user_id"`
		TaskID        int64  `db:"task_id"`
		PeriodKind    string `db:"period_kind"`
		PeriodStartMs int64  `db:"period_start_ms"`
	}
	if err := s.exec.QueryRowsCtx(ctx, &rows, `SELECT user_id, task_id, period_kind, period_start_ms
		FROM watch_send_reservation WHERE bucket_id=? ORDER BY period_kind, task_id FOR UPDATE`, bucketID); err != nil {
		return err
	}
	for _, row := range rows {
		query := `UPDATE watch_send_stat SET reserved_count=reserved_count-1 WHERE
			user_id=? AND task_id=? AND period_kind=? AND period_start_ms=? AND reserved_count>0`
		if delivered {
			query = `UPDATE watch_send_stat SET reserved_count=reserved_count-1, sent_count=sent_count+1 WHERE
				user_id=? AND task_id=? AND period_kind=? AND period_start_ms=? AND reserved_count>0`
		}
		res, err := s.exec.ExecCtx(ctx, query, row.UserID, row.TaskID, row.PeriodKind, row.PeriodStartMs)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil || affected != 1 {
			if err != nil {
				return err
			}
			return fmt.Errorf("watch quota reservation is inconsistent for bucket %d", bucketID)
		}
	}
	_, err := s.exec.ExecCtx(ctx, `DELETE FROM watch_send_reservation WHERE bucket_id=?`, bucketID)
	return err
}

func (s *SQLStore) commitLegacyWatchQuota(ctx context.Context, bucket *DeliveryBucket, nowMs int64) error {
	if bucket == nil {
		return nil
	}
	dayStart := nowMs / int64((24 * time.Hour).Milliseconds()) * int64((24 * time.Hour).Milliseconds())
	if err := s.IncrSent(ctx, bucket.UserID, 0, "day", dayStart); err != nil {
		return err
	}
	if len(bucket.HitIDs) == 0 {
		return nil
	}
	var taskIDs []int64
	args := append([]any{bucket.UserID}, intsToAny(bucket.HitIDs)...)
	if err := s.exec.QueryRowsCtx(ctx, &taskIDs, `SELECT DISTINCT task_id FROM watch_hit
		WHERE user_id=? AND id IN (`+placeholders(len(bucket.HitIDs))+`)`, args...); err != nil {
		return err
	}
	hourStart := nowMs / int64(time.Hour.Milliseconds()) * int64(time.Hour.Milliseconds())
	for _, taskID := range taskIDs {
		if err := s.IncrSent(ctx, bucket.UserID, taskID, "hour", hourStart); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLStore) CountSent(ctx context.Context, userID, taskID int64, periodKind string, periodStartMs int64) (int, error) {
	var row struct {
		N int64 `db:"n"`
	}
	if err := s.exec.QueryRowCtx(ctx, &row, `SELECT IFNULL(sent_count,0) AS n FROM watch_send_stat
		WHERE user_id=? AND task_id=? AND period_kind=? AND period_start_ms=?`, userID, taskID, periodKind, periodStartMs); err != nil {
		if err == sqlx.ErrNotFound {
			return 0, nil
		}
		return 0, err
	}
	return int(row.N), nil
}

func (s *SQLStore) IncrSent(ctx context.Context, userID, taskID int64, periodKind string, periodStartMs int64) error {
	_, err := s.exec.ExecCtx(ctx, `INSERT INTO watch_send_stat (user_id, task_id, period_kind, period_start_ms, sent_count)
		VALUES (?, ?, ?, ?, 1)
		ON DUPLICATE KEY UPDATE sent_count = sent_count + 1`, userID, taskID, periodKind, periodStartMs)
	return err
}
