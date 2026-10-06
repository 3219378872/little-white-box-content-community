package store

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"esx/pkg/adpolicy"
	"esx/pkg/event"
	"esx/pkg/idempotencyx"
	sqlx "esx/pkg/sqlstore"
)

// AdvertiserInput 是广告主申请或修改。
type AdvertiserInput struct {
	UserID           int64
	Name             string
	Markets          []string
	ExpectedRevision int64
	IdempotencyKey   string
}

// QualificationInput 是新增行业资质。
type QualificationInput struct {
	UserID           int64
	Market           string
	Industry         string
	DocumentAssetID  int64
	ValidUntilMs     int64
	ExpectedRevision int64
	IdempotencyKey   string
}

// AdvertiserWithQualifications 是广告主及其资质。
type AdvertiserWithQualifications struct {
	Advertiser     *Advertiser
	Qualifications []Qualification
}

// GetAdvertiserByUser 读取本人的广告主；不存在时返回 ErrNotFound。
func (s *Store) GetAdvertiserByUser(ctx context.Context, userID int64) (*AdvertiserWithQualifications, error) {
	return loadAdvertiser(ctx, s.conn, userID, false)
}

// loadAdvertiser 按用户载入广告主及其资质；lock=true 时加行锁，供同事务内的修改使用。
func loadAdvertiser(ctx context.Context, q rowQuerier, userID int64, lock bool) (*AdvertiserWithQualifications, error) {
	query := `SELECT ` + advertiserColumns + ` FROM advertiser WHERE user_id = ?`
	if lock {
		query += ` FOR UPDATE`
	}
	var adv Advertiser
	if err := q.QueryRowCtx(ctx, &adv, query, userID); err != nil {
		return nil, notFound(err)
	}
	var quals []Qualification
	if err := q.QueryRowsCtx(ctx, &quals, `SELECT `+qualificationColumns+` FROM advertiser_qualification
		WHERE advertiser_id = ? ORDER BY id`, adv.ID); err != nil {
		return nil, err
	}
	return &AdvertiserWithQualifications{Advertiser: &adv, Qualifications: quals}, nil
}

// ApplyAdvertiser 首次申请（ExpectedRevision=0）或修改资料；两者都产生新 revision 并送审（ADS-001）。
func (s *Store) ApplyAdvertiser(ctx context.Context, in AdvertiserInput, now time.Time) (*AdvertiserWithQualifications, error) {
	var out *AdvertiserWithQualifications
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		newID, err := s.nextID()
		if err != nil {
			return err
		}
		created, err := s.idempotent(ctx, session, "ad:advertiser:apply", in.UserID, in.IdempotencyKey, newID,
			in.Name, strings.Join(in.Markets, ","), strconv.FormatInt(in.ExpectedRevision, 10))
		if err != nil {
			return err
		}
		if !created {
			out, err = loadAdvertiser(ctx, session, in.UserID, false)
			return err
		}
		existing, err := loadAdvertiser(ctx, session, in.UserID, true)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		var adv *Advertiser
		if existing == nil {
			if in.ExpectedRevision != 0 {
				return ErrRevisionConflict
			}
			adv = &Advertiser{
				ID: newID, UserID: in.UserID, Name: in.Name, MarketsCSV: strings.Join(in.Markets, ","), Revision: 1,
				ReviewStatus: ReviewPending, PolicyCodesJSON: "[]", SubmittedAtMs: now.UnixMilli(),
				CreatedAtMs: now.UnixMilli(), UpdatedAtMs: now.UnixMilli(),
			}
			if _, err := session.ExecCtx(ctx, `INSERT INTO advertiser (`+advertiserColumns+`)
				VALUES (?, ?, ?, ?, ?, 0, ?, '[]', '', 0, ?, ?, ?)`,
				adv.ID, adv.UserID, adv.Name, adv.MarketsCSV, adv.Revision, adv.ReviewStatus,
				adv.SubmittedAtMs, adv.CreatedAtMs, adv.UpdatedAtMs); err != nil {
				if sqlx.IsDuplicateKey(err) {
					return ErrAdvertiserExists
				}
				return err
			}
		} else {
			if in.ExpectedRevision == 0 {
				return ErrAdvertiserExists
			}
			adv = existing.Advertiser
			if adv.Revision != in.ExpectedRevision {
				return ErrRevisionConflict
			}
			adv.Name, adv.MarketsCSV = in.Name, strings.Join(in.Markets, ",")
			if err := bumpAdvertiser(ctx, session, adv, now); err != nil {
				return err
			}
		}
		out, err = s.submitAdvertiser(ctx, session, adv, now)
		return err
	})
	return out, err
}

// AddQualification 新增资质并随广告主新 revision 一起送审（ADS-001、ADS-002）。
func (s *Store) AddQualification(ctx context.Context, in QualificationInput, now time.Time) (*AdvertiserWithQualifications, error) {
	var out *AdvertiserWithQualifications
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		qualID, err := s.nextID()
		if err != nil {
			return err
		}
		created, err := s.idempotent(ctx, session, "ad:qualification:add", in.UserID, in.IdempotencyKey, qualID,
			in.Market, in.Industry, strconv.FormatInt(in.DocumentAssetID, 10), strconv.FormatInt(in.ValidUntilMs, 10),
			strconv.FormatInt(in.ExpectedRevision, 10))
		if err != nil {
			return err
		}
		if !created {
			out, err = loadAdvertiser(ctx, session, in.UserID, false)
			return err
		}
		existing, err := loadAdvertiser(ctx, session, in.UserID, true)
		if err != nil {
			return err
		}
		adv := existing.Advertiser
		if adv.Revision != in.ExpectedRevision {
			return ErrRevisionConflict
		}
		if !slices.Contains(adv.Markets(), in.Market) {
			return ErrMarketNotAllowed
		}
		if err := requireAsset(ctx, session, in.UserID, in.DocumentAssetID, AssetDocument); err != nil {
			return err
		}
		if err := bumpAdvertiser(ctx, session, adv, now); err != nil {
			return err
		}
		if _, err := session.ExecCtx(ctx, `INSERT INTO advertiser_qualification (`+qualificationColumns+`)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			qualID, adv.ID, in.Market, in.Industry, in.DocumentAssetID, in.ValidUntilMs, QualificationPending,
			adv.Revision, now.UnixMilli(), now.UnixMilli()); err != nil {
			return err
		}
		out, err = s.submitAdvertiser(ctx, session, adv, now)
		return err
	})
	return out, err
}

// bumpAdvertiser 产生新 revision 并重置审核状态，修改后的广告主需要重新送审。
func bumpAdvertiser(ctx context.Context, session sqlx.Session, adv *Advertiser, now time.Time) error {
	adv.Revision++
	adv.ReviewStatus = ReviewPending
	adv.PolicyCodesJSON = "[]"
	adv.ReviewTaskID = 0
	adv.SubmittedAtMs = now.UnixMilli()
	adv.UpdatedAtMs = now.UnixMilli()
	_, err := session.ExecCtx(ctx, `UPDATE advertiser SET name = ?, markets = ?, revision = ?, review_status = ?,
		policy_codes = '[]', review_task_id = 0, submitted_at_ms = ?, updated_at_ms = ? WHERE id = ?`,
		adv.Name, adv.MarketsCSV, adv.Revision, adv.ReviewStatus, adv.SubmittedAtMs, adv.UpdatedAtMs, adv.ID)
	return err
}

// submitAdvertiser 在同一事务内写送审事件。资质对象以首个经营市场为审核市场。
func (s *Store) submitAdvertiser(ctx context.Context, session sqlx.Session, adv *Advertiser, now time.Time) (*AdvertiserWithQualifications, error) {
	full, err := loadAdvertiser(ctx, session, adv.UserID, false)
	if err != nil {
		return nil, err
	}
	sub := AdvertiserSubmission(full, now)
	if err := s.enqueueSubmission(ctx, session, sub); err != nil {
		return nil, err
	}
	return full, nil
}

// AdvertiserSubmission 由当前广告主与未失效资质构造送审载荷；对账补送复用同一构造。
func AdvertiserSubmission(full *AdvertiserWithQualifications, now time.Time) event.ReviewSubmittedEvent {
	adv := full.Advertiser
	markets := adv.Markets()
	market := ""
	if len(markets) > 0 {
		market = markets[0]
	}
	snap := event.ReviewSnapshot{
		Texts:  map[string]string{"name": adv.Name, "markets": adv.MarketsCSV},
		Market: market, Language: adpolicy.LanguageOf(market), SubmitterID: adv.ID,
	}
	for _, q := range full.Qualifications {
		if q.Status == QualificationRejected || q.Status == QualificationExpired {
			continue
		}
		snap.Qualifications = append(snap.Qualifications, event.ReviewQualification{
			QualificationID: q.ID, Market: q.Market, Industry: q.Industry,
			DocumentMediaID: q.DocumentMediaID, ValidUntilMs: q.ValidUntilMs,
		})
	}
	return event.ReviewSubmittedEvent{
		EventTime: now.UnixMilli(), BizType: event.ReviewBizAdvertiserQualification, ObjectID: adv.ID,
		Revision: adv.Revision, Purpose: event.ReviewPurposeInitial, SubmittedAt: adv.SubmittedAtMs, Snapshot: snap,
	}
}

// ValidQualification 报告广告主在（市场，行业）是否有有效的已过审资质（ADS-002）。
func ValidQualification(quals []Qualification, market, industry string, nowMs int64) bool {
	return slices.ContainsFunc(quals, func(q Qualification) bool {
		return q.Status == QualificationApproved && q.Market == market && q.Industry == industry && q.ValidUntilMs > nowMs
	})
}

// idempotent 为本次命令分配记录 ID 并解析幂等键，返回本次是否首次执行。
func (s *Store) idempotent(ctx context.Context, session sqlx.Session, scope string, userID int64, key string, resourceID int64, parts ...string) (bool, error) {
	recordID, err := s.nextID()
	if err != nil {
		return false, err
	}
	_, created, err := resolve(ctx, session, scope, userID, key, recordID, resourceID, parts...)
	return created, err
}

// resolve 绑定幂等键（ADS-013 沿用 CORE-050）；同键异命令返回 ErrIdempotencyConflict。
func resolve(ctx context.Context, session sqlx.Session, scope string, userID int64, key string, recordID, resourceID int64, parts ...string) (int64, bool, error) {
	return idempotencyx.ResolveIdempotencySession(ctx, session, idempotencyx.IdempotencyRecord{
		Scope: scope, UserID: userID, Key: key, CommandHash: idempotencyx.CommandHash(parts...),
	}, recordID, resourceID)
}
