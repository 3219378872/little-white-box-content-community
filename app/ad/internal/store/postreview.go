package store

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"esx/pkg/event"
	sqlx "esx/pkg/sqlstore"
)

// 举报原因（ADS-030）。只收结构化原因，不收自由文本，避免匿名举报携带个人信息。
var ReportReasons = []string{"misleading", "scam", "offensive", "inappropriate", "irrelevant", "other"}

// IsReportReason 报告举报原因是否受支持。
func IsReportReason(reason string) bool { return slices.Contains(ReportReasons, reason) }

// AppealTarget 返回可申诉的 revision（ADS-014）：最新 revision 被拒时申诉它；广告被下线且没有更新的
// 编辑时申诉被下线的过审 revision。每个 revision 只能申诉一次。
func AppealTarget(ad *Ad) (int64, bool) {
	target := int64(0)
	switch {
	case ad.ReviewStatus == ReviewRejected:
		target = ad.Revision
	case ad.ServingStatus == ServingOffline && ad.ApprovedRevision > 0 && ad.Revision == ad.ApprovedRevision &&
		ad.ReviewStatus == ReviewApproved:
		target = ad.ApprovedRevision
	}
	if target == 0 || ad.AppealedRevision >= target {
		return 0, false
	}
	return target, true
}

// AppealAd 发起申诉：状态转为 appealing，同事务写 appeal 送审事件；复审结论为最终结论（ADS-014）。
// 同一幂等键重复提交返回当前广告。
func (s *Store) AppealAd(ctx context.Context, userID, adID int64, idempotencyKey string, now time.Time) (*AdWithSnapshots, error) {
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		recordID, err := s.nextID()
		if err != nil {
			return err
		}
		_, created, err := resolveIdempotency(ctx, session, "ad:appeal", userID, idempotencyKey, recordID, adID,
			strconv.FormatInt(adID, 10))
		if err != nil || !created {
			return err
		}
		var ad Ad
		if err := session.QueryRowCtx(ctx, &ad, `SELECT `+adColumns+` FROM ad WHERE id = ? AND user_id = ? FOR UPDATE`,
			adID, userID); err != nil {
			return notFound(err)
		}
		target, ok := AppealTarget(&ad)
		if !ok {
			return ErrAppealNotAllowed
		}
		ad.ReviewStatus, ad.AppealedRevision = ReviewAppealing, target
		ad.SubmittedAtMs, ad.UpdatedAtMs = now.UnixMilli(), now.UnixMilli()
		if _, err := session.ExecCtx(ctx, `UPDATE ad SET review_status = ?, appealed_revision = ?, review_task_id = 0,
			submitted_at_ms = ?, updated_at_ms = ? WHERE id = ?`,
			ad.ReviewStatus, ad.AppealedRevision, ad.SubmittedAtMs, ad.UpdatedAtMs, ad.ID); err != nil {
			return err
		}
		snap, err := snapshotIn(ctx, session, ad.ID, target)
		if err != nil {
			return err
		}
		return s.enqueueSubmission(ctx, session, AdSubmission(&ad, snap, event.ReviewPurposeAppeal, now))
	})
	if err != nil {
		return nil, err
	}
	return s.GetAd(ctx, userID, adID)
}

// ReportInput 是一次举报。
type ReportInput struct {
	AdID        int64
	ReporterKey string
	Reason      string
}

// ReportResult 说明举报是否被计入。
type ReportResult struct {
	// Counted 为 false 表示同一身份重复举报或广告已下线，未新增计数
	Counted bool
	Count   int
}

// ReportPriority 随举报数提高举报复审任务的优先级（ADS-030）。
func ReportPriority(count int) int32 {
	return int32(min(1000, 40+10*count))
}

// ReportAd 记录举报并经 outbox 送 report 复审任务（ADS-030）。同一身份对同一广告只计一次；
// 未决批次内的举报共用一个任务，举报越多优先级越高。举报针对正在投放的过审快照。
func (s *Store) ReportAd(ctx context.Context, in ReportInput, now time.Time) (ReportResult, error) {
	var result ReportResult
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		var ad Ad
		if err := session.QueryRowCtx(ctx, &ad, `SELECT `+adColumns+` FROM ad WHERE id = ? FOR UPDATE`, in.AdID); err != nil {
			return notFound(err)
		}
		if ad.ApprovedRevision == 0 {
			return ErrNotFound // 从未过审的广告不会出现在推荐流中
		}
		if ad.ServingStatus == ServingOffline {
			return nil
		}
		reportID, err := s.nextID()
		if err != nil {
			return err
		}
		batch := ad.ReportBatch
		if batch == "" {
			batch = strconv.FormatInt(reportID, 10)
		}
		inserted, err := session.ExecCtx(ctx, `INSERT IGNORE INTO ad_report
			(id, ad_id, revision, reporter_key, reason, batch, created_at_ms) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			reportID, ad.ID, ad.ApprovedRevision, in.ReporterKey, in.Reason, batch, now.UnixMilli())
		if err != nil {
			return err
		}
		if n, err := inserted.RowsAffected(); err != nil || n == 0 {
			return err
		}
		if ad.ReportBatch == "" {
			if _, err := session.ExecCtx(ctx, `UPDATE ad SET report_batch = ? WHERE id = ?`, batch, ad.ID); err != nil {
				return err
			}
		}
		count, err := s.submitReportBatch(ctx, session, &ad, batch, now)
		if err != nil {
			return err
		}
		result = ReportResult{Counted: true, Count: count}
		return nil
	})
	return result, err
}

// submitReportBatch 以批次举报数为优先级送审；审核平台对同一批次只建一个任务并提高其优先级。
func (s *Store) submitReportBatch(ctx context.Context, session sqlx.Session, ad *Ad, batch string, now time.Time) (int, error) {
	var count int
	if err := session.QueryRowCtx(ctx, &count, `SELECT COUNT(*) FROM ad_report WHERE ad_id = ? AND batch = ?`,
		ad.ID, batch); err != nil {
		return 0, err
	}
	snap, err := snapshotIn(ctx, session, ad.ID, ad.ApprovedRevision)
	if err != nil {
		return 0, err
	}
	sub := AdSubmission(ad, snap, event.ReviewPurposeReport, now)
	sub.PurposeKey, sub.Priority, sub.SubmittedAt = batch, ReportPriority(count), now.UnixMilli()
	return count, s.enqueueSubmission(ctx, session, sub)
}

// CloseReportBatch 在举报复审结论到达后关闭批次（按 purposeKey CAS，重复投递无副作用）。
// 结论作出后、应用之前到达的举报仍挂在旧批次上，审核平台不会再为它们建任务；若举报未成立且广告仍在投，
// 把这些举报移入新批次重新送审，避免静默丢失。
func (s *Store) CloseReportBatch(ctx context.Context, d event.ReviewDecidedEvent, now time.Time) error {
	if d.Purpose != event.ReviewPurposeReport || d.PurposeKey == "" {
		return nil
	}
	return s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		var ad Ad
		if err := session.QueryRowCtx(ctx, &ad, `SELECT `+adColumns+` FROM ad WHERE id = ? FOR UPDATE`, d.ObjectID); err != nil {
			if errors.Is(notFound(err), ErrNotFound) {
				return nil
			}
			return err
		}
		if ad.ReportBatch != d.PurposeKey {
			return nil
		}
		var late []int64
		if d.Verdict == event.ReviewVerdictApprove && ad.ServingStatus != ServingOffline && ad.ApprovedRevision > 0 {
			if err := session.QueryRowsCtx(ctx, &late, `SELECT id FROM ad_report WHERE ad_id = ? AND batch = ?
				AND created_at_ms > ? ORDER BY id`, ad.ID, d.PurposeKey, d.DecidedAt); err != nil {
				return err
			}
		}
		next := ""
		if len(late) > 0 {
			next = strconv.FormatInt(late[0], 10)
			if _, err := session.ExecCtx(ctx, `UPDATE ad_report SET batch = ? WHERE ad_id = ? AND batch = ? AND created_at_ms > ?`,
				next, ad.ID, d.PurposeKey, d.DecidedAt); err != nil {
				return err
			}
		}
		if _, err := session.ExecCtx(ctx, `UPDATE ad SET report_batch = ? WHERE id = ?`, next, ad.ID); err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		_, err := s.submitReportBatch(ctx, session, &ad, next, now)
		return err
	})
}

// RescanCandidates 返回尚未处理当前回扫代次的在投广告（ADS-031）。已暂停的广告正在等待人审，不重复回扫。
func (s *Store) RescanCandidates(ctx context.Context, generation string, limit int) ([]Ad, error) {
	var ads []Ad
	err := s.conn.QueryRowsCtx(ctx, &ads, `SELECT `+adColumns+` FROM ad
		WHERE serving_status = ? AND approved_revision > 0 AND rescan_generation <> ?
		ORDER BY id LIMIT ?`, ServingServing, generation, limit)
	return ads, err
}

// MarkRescanned 记录过审结论已按该代次作出、无需回扫的广告；只在过审 revision 未变化时写入。
func (s *Store) MarkRescanned(ctx context.Context, adID, approvedRevision int64, generation string) error {
	_, err := s.exec(ctx, `UPDATE ad SET rescan_generation = ? WHERE id = ? AND approved_revision = ?`,
		generation, adID, approvedRevision)
	return err
}

// SubmitRescan 把在投的过审快照送 rescan 任务，并在同一事务内记录代次（ADS-031）。
// 广告在读取后被暂停、下线或切换过审版本时跳过，返回 false。
func (s *Store) SubmitRescan(ctx context.Context, adID, approvedRevision int64, generation string, now time.Time) (bool, error) {
	submitted := false
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		var ad Ad
		if err := session.QueryRowCtx(ctx, &ad, `SELECT `+adColumns+` FROM ad WHERE id = ? FOR UPDATE`, adID); err != nil {
			return notFound(err)
		}
		if ad.ServingStatus != ServingServing || ad.ApprovedRevision != approvedRevision || ad.RescanGeneration == generation {
			return nil
		}
		snap, err := snapshotIn(ctx, session, ad.ID, ad.ApprovedRevision)
		if err != nil {
			return err
		}
		sub := AdSubmission(&ad, snap, event.ReviewPurposeRescan, now)
		sub.PurposeKey, sub.SubmittedAt = generation, now.UnixMilli()
		if err := s.enqueueSubmission(ctx, session, sub); err != nil {
			return err
		}
		if _, err := session.ExecCtx(ctx, `UPDATE ad SET rescan_generation = ? WHERE id = ?`, generation, ad.ID); err != nil {
			return err
		}
		submitted = true
		return nil
	})
	return submitted, err
}

func snapshotIn(ctx context.Context, session sqlx.Session, adID, revision int64) (*Snapshot, error) {
	var snap Snapshot
	if err := session.QueryRowCtx(ctx, &snap, `SELECT `+snapshotColumns+` FROM ad_snapshot WHERE ad_id = ? AND revision = ?`,
		adID, revision); err != nil {
		return nil, notFound(err)
	}
	return &snap, nil
}
