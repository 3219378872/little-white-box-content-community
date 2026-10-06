package store

import (
	"context"
	"time"

	"esx/pkg/event"
)

// DecisionEffect 描述一次结论应用对投放的影响。
type DecisionEffect struct {
	Applied        bool
	ServingChanged bool
	AdID           int64
	UserID         int64
}

// ApplyAdDecision 按（objectId, revision）CAS 应用审核结论（RVW-040）。
//
// 乱序或重复到达的旧 revision 结论影响 0 行，只留在审核平台的审计中。
//   - 送审通过只前移 approved_revision，旧过审快照在新版本审核期间继续投放（ADS-011）；新版本过审时
//     解除旧快照的回扫暂停。
//   - 投后任务（质检、举报、回扫）针对 approved_revision：拒绝即下线（RVW-014、ADS-030、ADS-031），
//     回扫的暂停结论先停投（Interim），回扫人审否定后恢复投放。
//   - 申诉复审是最终结论（ADS-014）：通过则该 revision 成为过审版本并恢复投放，拒绝维持原状。
func (s *Store) ApplyAdDecision(ctx context.Context, d event.ReviewDecidedEvent, now time.Time) (DecisionEffect, error) {
	effect := DecisionEffect{AdID: d.ObjectID}
	codes := encodeCodes(d.PolicyCodes)
	approve := d.Verdict == event.ReviewVerdictApprove
	var affected int64
	var err error
	switch {
	case d.Interim:
		affected, err = s.exec(ctx, `UPDATE ad SET serving_status = ?, pause_reason = ?, policy_codes = ?, updated_at_ms = ?
			WHERE id = ? AND approved_revision = ? AND serving_status = ?`,
			ServingPaused, PauseRescan, codes, now.UnixMilli(), d.ObjectID, d.Revision, ServingServing)
		effect.ServingChanged = affected > 0
	case isPostServing(d.Purpose) && !approve:
		affected, err = s.exec(ctx, `UPDATE ad SET serving_status = ?, pause_reason = ?, policy_codes = ?,
			review_status = IF(revision = ?, ?, review_status), updated_at_ms = ?
			WHERE id = ? AND approved_revision = ? AND serving_status <> ?`,
			ServingOffline, d.Purpose, codes, d.Revision, ReviewRejected, now.UnixMilli(),
			d.ObjectID, d.Revision, ServingOffline)
		effect.ServingChanged = affected > 0
	case d.Purpose == event.ReviewPurposeRescan:
		affected, err = s.exec(ctx, `UPDATE ad SET serving_status = ?, pause_reason = '',
			policy_codes = IF(review_status = ?, '[]', policy_codes), updated_at_ms = ?
			WHERE id = ? AND approved_revision = ? AND serving_status = ? AND pause_reason = ?`,
			ServingServing, ReviewApproved, now.UnixMilli(), d.ObjectID, d.Revision, ServingPaused, PauseRescan)
		effect.ServingChanged = affected > 0
	case isPostServing(d.Purpose):
		return effect, nil
	case d.Purpose == event.ReviewPurposeAppeal && approve:
		affected, err = s.exec(ctx, `UPDATE ad SET approved_revision = ?, approved_at_ms = ?, review_status = ?,
			policy_codes = '[]', serving_status = ?, pause_reason = '', updated_at_ms = ?
			WHERE id = ? AND revision = ? AND review_status = ? AND appealed_revision = ?`,
			d.Revision, d.DecidedAt, ReviewApproved, ServingServing, now.UnixMilli(),
			d.ObjectID, d.Revision, ReviewAppealing, d.Revision)
		effect.ServingChanged = affected > 0
	case d.Purpose == event.ReviewPurposeAppeal:
		affected, err = s.exec(ctx, `UPDATE ad SET review_status = ?, policy_codes = ?, updated_at_ms = ?
			WHERE id = ? AND revision = ? AND review_status = ? AND appealed_revision = ?`,
			ReviewRejected, codes, now.UnixMilli(), d.ObjectID, d.Revision, ReviewAppealing, d.Revision)
	case approve:
		// MySQL 按书写顺序赋值，pause_reason 读取的是已更新的 serving_status。
		affected, err = s.exec(ctx, `UPDATE ad SET approved_revision = ?, approved_at_ms = ?, review_status = ?,
			policy_codes = '[]',
			serving_status = CASE WHEN serving_status = ? OR (serving_status = ? AND pause_reason = ?) THEN ? ELSE serving_status END,
			pause_reason = IF(serving_status = ? AND pause_reason = ?, '', pause_reason),
			updated_at_ms = ?
			WHERE id = ? AND revision = ? AND approved_revision < ?`,
			d.Revision, d.DecidedAt, ReviewApproved,
			ServingNone, ServingPaused, PauseRescan, ServingServing,
			ServingServing, PauseRescan,
			now.UnixMilli(), d.ObjectID, d.Revision, d.Revision)
		effect.ServingChanged = affected > 0
	default:
		affected, err = s.exec(ctx, `UPDATE ad SET review_status = ?, policy_codes = ?, updated_at_ms = ?
			WHERE id = ? AND revision = ? AND review_status = ?`,
			ReviewRejected, codes, now.UnixMilli(), d.ObjectID, d.Revision, ReviewPending)
	}
	if err != nil {
		return effect, err
	}
	effect.Applied = affected > 0
	if effect.Applied {
		var owner int64
		if err := s.conn.QueryRowCtx(ctx, &owner, `SELECT user_id FROM ad WHERE id = ?`, d.ObjectID); err == nil {
			effect.UserID = owner
		}
	}
	return effect, nil
}

// isPostServing 判断送审目的是否属于投放后复审（抽检、举报、复扫），这类结论只影响投放状态，不改过审版本。
func isPostServing(purpose string) bool {
	return purpose == event.ReviewPurposeQA || purpose == event.ReviewPurposeReport || purpose == event.ReviewPurposeRescan
}

// ApplyAdvertiserDecision 应用资质对象结论；随该 revision 送审的资质一并更新。
func (s *Store) ApplyAdvertiserDecision(ctx context.Context, d event.ReviewDecidedEvent, now time.Time) (DecisionEffect, error) {
	effect := DecisionEffect{}
	if d.Purpose != event.ReviewPurposeInitial && d.Purpose != event.ReviewPurposeAppeal {
		return effect, nil
	}
	var affected int64
	var err error
	if d.Verdict == event.ReviewVerdictApprove {
		affected, err = s.exec(ctx, `UPDATE advertiser SET approved_revision = ?, review_status = ?, approved_name = name,
			policy_codes = '[]', updated_at_ms = ? WHERE id = ? AND revision = ? AND approved_revision < ?`,
			d.Revision, ReviewApproved, now.UnixMilli(), d.ObjectID, d.Revision, d.Revision)
		if err == nil && affected > 0 {
			_, err = s.exec(ctx, `UPDATE advertiser_qualification SET status = ?, updated_at_ms = ?
				WHERE advertiser_id = ? AND submitted_revision <= ? AND status = ?`,
				QualificationApproved, now.UnixMilli(), d.ObjectID, d.Revision, QualificationPending)
		}
	} else {
		affected, err = s.exec(ctx, `UPDATE advertiser SET review_status = ?, policy_codes = ?, updated_at_ms = ?
			WHERE id = ? AND revision = ? AND review_status IN (?, ?)`,
			ReviewRejected, encodeCodes(d.PolicyCodes), now.UnixMilli(), d.ObjectID, d.Revision, ReviewPending, ReviewAppealing)
		if err == nil && affected > 0 {
			_, err = s.exec(ctx, `UPDATE advertiser_qualification SET status = ?, updated_at_ms = ?
				WHERE advertiser_id = ? AND submitted_revision = ? AND status = ?`,
				QualificationRejected, now.UnixMilli(), d.ObjectID, d.Revision, QualificationPending)
		}
	}
	effect.Applied = affected > 0
	return effect, err
}

// exec 执行语句并返回影响行数，调用方据此判断结论是否已过期。
func (s *Store) exec(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := s.conn.ExecCtx(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
