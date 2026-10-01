package store

import (
	"context"
	"time"
)

// ReconcileGrace 与 RVW-041 一致：送审 5 分钟后仍无任务的对象必须补送。
const ReconcileGrace = 5 * time.Minute

// PendingAdsWithoutTask 返回送审超过宽限期、尚未登记审核任务的广告。
func (s *Store) PendingAdsWithoutTask(ctx context.Context, now time.Time, limit int) ([]Ad, error) {
	var ads []Ad
	err := s.conn.QueryRowsCtx(ctx, &ads, `SELECT `+adColumns+` FROM ad
		WHERE review_status IN (?, ?) AND review_task_id = 0 AND submitted_at_ms < ?
		ORDER BY submitted_at_ms LIMIT ?`, ReviewPending, ReviewAppealing, now.Add(-ReconcileGrace).UnixMilli(), limit)
	return ads, err
}

// PendingAdvertisersWithoutTask 同上，针对资质对象。
func (s *Store) PendingAdvertisersWithoutTask(ctx context.Context, now time.Time, limit int) ([]Advertiser, error) {
	var advertisers []Advertiser
	err := s.conn.QueryRowsCtx(ctx, &advertisers, `SELECT `+advertiserColumns+` FROM advertiser
		WHERE review_status = ? AND review_task_id = 0 AND submitted_at_ms < ?
		ORDER BY submitted_at_ms LIMIT ?`, ReviewPending, now.Add(-ReconcileGrace).UnixMilli(), limit)
	return advertisers, err
}

// RecordAdTask 记录审核平台确认的任务；只在 revision 未变化时写入。
func (s *Store) RecordAdTask(ctx context.Context, adID, revision, taskID int64) error {
	_, err := s.exec(ctx, `UPDATE ad SET review_task_id = ? WHERE id = ? AND revision = ? AND review_task_id = 0`, taskID, adID, revision)
	return err
}

func (s *Store) RecordAdvertiserTask(ctx context.Context, advertiserID, revision, taskID int64) error {
	_, err := s.exec(ctx, `UPDATE advertiser SET review_task_id = ? WHERE id = ? AND revision = ? AND review_task_id = 0`,
		taskID, advertiserID, revision)
	return err
}

// AdvertiserByID 读取广告主（对账用）。
func (s *Store) AdvertiserByID(ctx context.Context, id int64) (*AdvertiserWithQualifications, error) {
	var userID int64
	if err := s.conn.QueryRowCtx(ctx, &userID, `SELECT user_id FROM advertiser WHERE id = ?`, id); err != nil {
		return nil, notFound(err)
	}
	return loadAdvertiser(ctx, s.conn, userID, false)
}

// ExpireQualifications 把到期资质标为 expired，返回受影响的广告主（ADS-002）。
func (s *Store) ExpireQualifications(ctx context.Context, now time.Time) ([]Qualification, error) {
	var expiring []Qualification
	if err := s.conn.QueryRowsCtx(ctx, &expiring, `SELECT `+qualificationColumns+` FROM advertiser_qualification
		WHERE status = ? AND valid_until_ms <= ? LIMIT 200`, QualificationApproved, now.UnixMilli()); err != nil {
		return nil, err
	}
	var out []Qualification
	for _, q := range expiring {
		affected, err := s.exec(ctx, `UPDATE advertiser_qualification SET status = ?, updated_at_ms = ?
			WHERE id = ? AND status = ?`, QualificationExpired, now.UnixMilli(), q.ID, QualificationApproved)
		if err != nil {
			return out, err
		}
		if affected > 0 {
			out = append(out, q)
		}
	}
	return out, nil
}

// MarkQualificationLapsed 给依赖失效资质的广告标记原因，供广告主查看（ADS-002，以 INDUSTRY.QUALIFICATION 告知）。
func (s *Store) MarkQualificationLapsed(ctx context.Context, q Qualification, now time.Time) ([]int64, error) {
	var ids []int64
	if err := s.conn.QueryRowsCtx(ctx, &ids, `SELECT id FROM ad WHERE advertiser_id = ? AND market = ? AND industry = ?`,
		q.AdvertiserID, q.Market, q.Industry); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	_, err := s.exec(ctx, `UPDATE ad SET pause_reason = 'INDUSTRY.QUALIFICATION', updated_at_ms = ?
		WHERE advertiser_id = ? AND market = ? AND industry = ?`, now.UnixMilli(), q.AdvertiserID, q.Market, q.Industry)
	return ids, err
}

// MarkPublished 记录过审素材已复制到公开路径。
func (s *Store) MarkPublished(ctx context.Context, adID, revision int64) error {
	_, err := s.exec(ctx, `UPDATE ad SET published_revision = ? WHERE id = ? AND published_revision < ?`, revision, adID, revision)
	return err
}

// ServingCandidates 返回所有可能投放的广告，用于重建投放索引。
func (s *Store) ServingCandidates(ctx context.Context) ([]Ad, error) {
	var ads []Ad
	err := s.conn.QueryRowsCtx(ctx, &ads, `SELECT `+adColumns+` FROM ad WHERE serving_status = ? AND approved_revision > 0`,
		ServingServing)
	return ads, err
}

// AdByID 读取广告（ad-mq 与投放使用，不做归属校验）。
func (s *Store) AdByID(ctx context.Context, id int64) (*Ad, error) {
	var ad Ad
	if err := s.conn.QueryRowCtx(ctx, &ad, `SELECT `+adColumns+` FROM ad WHERE id = ?`, id); err != nil {
		return nil, notFound(err)
	}
	return &ad, nil
}

// QualificationsOf 读取广告主资质。
func (s *Store) QualificationsOf(ctx context.Context, advertiserID int64) ([]Qualification, error) {
	var quals []Qualification
	err := s.conn.QueryRowsCtx(ctx, &quals, `SELECT `+qualificationColumns+` FROM advertiser_qualification
		WHERE advertiser_id = ?`, advertiserID)
	return quals, err
}
