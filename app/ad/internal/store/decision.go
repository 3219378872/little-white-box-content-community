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
// 乱序或重复到达的旧 revision 结论影响 0 行，只留在审核平台的审计中；通过时只前移
// approved_revision，旧过审快照在新版本审核期间继续投放（ADS-011）。质检判定违规时
// 对象立即停止投放（RVW-014）。
func (s *Store) ApplyAdDecision(ctx context.Context, d event.ReviewDecidedEvent, now time.Time) (DecisionEffect, error) {
	effect := DecisionEffect{AdID: d.ObjectID}
	codes := encodeCodes(d.PolicyCodes)
	var affected int64
	var err error
	switch {
	case d.Purpose == event.ReviewPurposeQA && d.Verdict == event.ReviewVerdictReject:
		affected, err = s.exec(ctx, `UPDATE ad SET serving_status = ?, pause_reason = 'qa', policy_codes = ?,
			review_status = IF(revision = ?, ?, review_status), updated_at_ms = ?
			WHERE id = ? AND approved_revision = ? AND serving_status <> ?`,
			ServingOffline, codes, d.Revision, ReviewRejected, now.UnixMilli(), d.ObjectID, d.Revision, ServingOffline)
		effect.ServingChanged = affected > 0
	case d.Purpose == event.ReviewPurposeQA:
		return effect, nil
	case d.Verdict == event.ReviewVerdictApprove:
		affected, err = s.exec(ctx, `UPDATE ad SET approved_revision = ?, review_status = ?, policy_codes = '[]',
			serving_status = IF(serving_status = ?, ?, serving_status), updated_at_ms = ?
			WHERE id = ? AND revision = ? AND approved_revision < ?`,
			d.Revision, ReviewApproved, ServingNone, ServingServing, now.UnixMilli(), d.ObjectID, d.Revision, d.Revision)
		effect.ServingChanged = affected > 0
	default:
		affected, err = s.exec(ctx, `UPDATE ad SET review_status = ?, policy_codes = ?, updated_at_ms = ?
			WHERE id = ? AND revision = ? AND review_status IN (?, ?)`,
			ReviewRejected, codes, now.UnixMilli(), d.ObjectID, d.Revision, ReviewPending, ReviewAppealing)
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

func (s *Store) exec(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := s.conn.ExecCtx(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
