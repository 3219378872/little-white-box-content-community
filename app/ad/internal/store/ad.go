package store

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"esx/pkg/adpolicy"
	"esx/pkg/event"
	"esx/pkg/mqx"
	"esx/pkg/outboxx"
	sqlx "esx/pkg/sqlstore"
)

// AdInput 是创建或编辑广告的内容；校验由调用方完成。
type AdInput struct {
	Title         string
	Body          string
	CTA           string
	LandingURL    string
	LandingDomain string
	MediaIDs      []int64
	Market        string
	Industry      string
	StartMs       int64
	EndMs         int64
}

// AdWithSnapshots 是广告及其最新与过审快照。
type AdWithSnapshots struct {
	Ad       *Ad
	Latest   *Snapshot
	Approved *Snapshot
}

// CreateAd 创建广告并自动送审（ADS-001、ADS-002、ADS-011）。
func (s *Store) CreateAd(ctx context.Context, userID int64, in AdInput, idempotencyKey string, now time.Time) (*AdWithSnapshots, error) {
	var adID int64
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		newID, err := s.nextID()
		if err != nil {
			return err
		}
		recordID, err := s.nextID()
		if err != nil {
			return err
		}
		resourceID, created, err := resolveIdempotency(ctx, session, "ad:create", userID, idempotencyKey, recordID, newID, commandParts(0, in)...)
		if err != nil {
			return err
		}
		adID = resourceID
		if !created {
			return nil
		}
		adv, err := s.advertiserForAd(ctx, session, userID, in, now)
		if err != nil {
			return err
		}
		ad := &Ad{
			ID: newID, AdvertiserID: adv.ID, UserID: userID, Revision: 1, ReviewStatus: ReviewPending,
			ServingStatus: ServingNone, Market: in.Market, Industry: in.Industry, StartMs: in.StartMs, EndMs: in.EndMs,
			PolicyCodesJSON: "[]", SubmittedAtMs: now.UnixMilli(), CreatedAtMs: now.UnixMilli(), UpdatedAtMs: now.UnixMilli(),
		}
		if _, err := session.ExecCtx(ctx, `INSERT INTO ad (`+adColumns+`)
			VALUES (?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, '[]', '', 0, 0, ?, 0, 0, '', '', ?, ?)`,
			ad.ID, ad.AdvertiserID, ad.UserID, ad.Revision, ad.ReviewStatus, ad.ServingStatus, ad.Market, ad.Industry,
			ad.StartMs, ad.EndMs, ad.SubmittedAtMs, ad.CreatedAtMs, ad.UpdatedAtMs); err != nil {
			return err
		}
		return s.writeRevision(ctx, session, ad, adv, in, now)
	})
	if err != nil {
		return nil, err
	}
	return s.GetAd(ctx, userID, adID)
}

// UpdateAd 产生新 revision 并送审；approved_revision 不变，审核期间继续投放旧过审快照（ADS-011）。
func (s *Store) UpdateAd(ctx context.Context, userID, adID, expectedRevision int64, in AdInput, idempotencyKey string, now time.Time) (*AdWithSnapshots, error) {
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		recordID, err := s.nextID()
		if err != nil {
			return err
		}
		_, created, err := resolveIdempotency(ctx, session, "ad:update", userID, idempotencyKey, recordID, adID,
			commandParts(expectedRevision, in, strconv.FormatInt(adID, 10))...)
		if err != nil || !created {
			return err
		}
		var ad Ad
		if err := session.QueryRowCtx(ctx, &ad, `SELECT `+adColumns+` FROM ad WHERE id = ? AND user_id = ? FOR UPDATE`, adID, userID); err != nil {
			return notFound(err)
		}
		if ad.Revision != expectedRevision {
			return ErrRevisionConflict
		}
		adv, err := s.advertiserForAd(ctx, session, userID, in, now)
		if err != nil {
			return err
		}
		ad.Revision++
		ad.ReviewStatus = ReviewPending
		ad.Market, ad.Industry, ad.StartMs, ad.EndMs = in.Market, in.Industry, in.StartMs, in.EndMs
		ad.SubmittedAtMs, ad.UpdatedAtMs = now.UnixMilli(), now.UnixMilli()
		if _, err := session.ExecCtx(ctx, `UPDATE ad SET revision = ?, review_status = ?, market = ?, industry = ?,
			start_ms = ?, end_ms = ?, policy_codes = '[]', review_task_id = 0, submitted_at_ms = ?, updated_at_ms = ?
			WHERE id = ?`, ad.Revision, ad.ReviewStatus, ad.Market, ad.Industry, ad.StartMs, ad.EndMs,
			ad.SubmittedAtMs, ad.UpdatedAtMs, ad.ID); err != nil {
			return err
		}
		return s.writeRevision(ctx, session, &ad, adv, in, now)
	})
	if err != nil {
		return nil, err
	}
	return s.GetAd(ctx, userID, adID)
}

// advertiserForAd 校验广告主已过审、市场在经营范围内、受监管行业具备有效资质（ADS-001、ADS-002、ADS-A08）。
// 年龄限制行业允许送审，由审核平台硬规则以对应政策码拒绝（ADS-017）。
func (s *Store) advertiserForAd(ctx context.Context, session sqlx.Session, userID int64, in AdInput, now time.Time) (*Advertiser, error) {
	full, err := loadAdvertiser(ctx, session, userID, false)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrAdvertiserNotApproved
	}
	if err != nil {
		return nil, err
	}
	adv := full.Advertiser
	if adv.ApprovedRevision == 0 || adv.ApprovedName == "" {
		return nil, ErrAdvertiserNotApproved
	}
	if !slices.Contains(adv.Markets(), in.Market) {
		return nil, ErrMarketNotAllowed
	}
	if adpolicy.DispositionOf(in.Market, in.Industry).NeedsQualification &&
		!ValidQualification(full.Qualifications, in.Market, in.Industry, now.UnixMilli()) {
		return nil, ErrQualificationMissing
	}
	for _, id := range in.MediaIDs {
		if err := requireAsset(ctx, session, userID, id, AssetCreative); err != nil {
			return nil, err
		}
	}
	return adv, nil
}

// writeRevision 写只读快照与送审事件（ADS-012：投放的广告主名称取自过审的主体名称）。
func (s *Store) writeRevision(ctx context.Context, session sqlx.Session, ad *Ad, adv *Advertiser, in AdInput, now time.Time) error {
	media := make([]SnapshotMedia, 0, len(in.MediaIDs))
	for _, id := range in.MediaIDs {
		var sha string
		if err := session.QueryRowCtx(ctx, &sha, `SELECT sha256 FROM ad_asset WHERE id = ?`, id); err != nil {
			return notFound(err)
		}
		media = append(media, SnapshotMedia{MediaID: id, SHA256: sha})
	}
	rawMedia, err := json.Marshal(media)
	if err != nil {
		return err
	}
	snap := &Snapshot{
		AdID: ad.ID, Revision: ad.Revision, AdvertiserName: adv.ApprovedName, Title: in.Title, Body: in.Body,
		CTA: in.CTA, LandingURL: in.LandingURL, LandingDomain: in.LandingDomain, MediaJSON: string(rawMedia),
		Market: in.Market, Language: adpolicy.LanguageOf(in.Market), Industry: in.Industry, CreatedAtMs: now.UnixMilli(),
	}
	if _, err := session.ExecCtx(ctx, `INSERT INTO ad_snapshot (`+snapshotColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.AdID, snap.Revision, snap.AdvertiserName, snap.Title, snap.Body, snap.CTA, snap.LandingURL,
		snap.LandingDomain, snap.MediaJSON, snap.Market, snap.Language, snap.Industry, snap.CreatedAtMs); err != nil {
		return err
	}
	return s.enqueueSubmission(ctx, session, AdSubmission(ad, snap, event.ReviewPurposeInitial, now))
}

// AdSubmission 由广告与快照构造送审载荷；对账补送复用同一构造，保证同一 revision 的快照一致。
func AdSubmission(ad *Ad, snap *Snapshot, purpose string, now time.Time) event.ReviewSubmittedEvent {
	reviewSnap := event.ReviewSnapshot{
		Texts:      map[string]string{"title": snap.Title, "body": snap.Body, "cta": snap.CTA, "advertiser": snap.AdvertiserName},
		LandingURL: snap.LandingURL, Market: snap.Market, Language: snap.Language, Industry: snap.Industry,
		SubmitterID: ad.AdvertiserID,
	}
	for _, m := range snap.Media() {
		reviewSnap.Media = append(reviewSnap.Media, event.ReviewMedia{MediaID: m.MediaID, SHA256: m.SHA256, Kind: "image"})
	}
	return event.ReviewSubmittedEvent{
		EventTime: now.UnixMilli(), BizType: event.ReviewBizAdCreative, ObjectID: ad.ID, Revision: snap.Revision,
		Purpose: purpose, SubmittedAt: ad.SubmittedAtMs, Snapshot: reviewSnap,
	}
}

// enqueueSubmission 在当前事务内写入送审事件；事件 ID 同时是 outbox 幂等键，消息 Key 只用于追踪。
func (s *Store) enqueueSubmission(ctx context.Context, session sqlx.Session, sub event.ReviewSubmittedEvent) error {
	id, err := s.nextID()
	if err != nil {
		return err
	}
	sub.EventID = id
	payload, err := sub.MarshalPayload()
	if err != nil {
		return err
	}
	key := "review-submitted:" + sub.BizType + ":" + strconv.FormatInt(sub.ObjectID, 10) + ":" +
		strconv.FormatInt(sub.Revision, 10) + ":" + sub.Purpose
	if sub.PurposeKey != "" {
		// 同一批举报的后续送审只提高优先级，键带上优先级以免被当作重复消息。
		key += ":" + sub.PurposeKey + ":" + strconv.FormatInt(int64(sub.Priority), 10)
	}
	if len(key) > 128 { // message_key VARCHAR(128)；键只用于追踪，不承担去重
		key = key[:128]
	}
	return s.outbox.Enqueue(ctx, session, outboxx.Event{
		ID: id, Topic: mqx.TopicReviewSubmitted, Tag: sub.BizType, Key: key, Payload: payload,
	})
}

// GetAd 读取本人广告；越权统一返回不存在（ADS-040）。
func (s *Store) GetAd(ctx context.Context, userID, adID int64) (*AdWithSnapshots, error) {
	var ad Ad
	if err := s.conn.QueryRowCtx(ctx, &ad, `SELECT `+adColumns+` FROM ad WHERE id = ? AND user_id = ?`, adID, userID); err != nil {
		return nil, notFound(err)
	}
	return s.withSnapshots(ctx, &ad)
}

// ListAds 按 ID 倒序分页列出本人广告。
func (s *Store) ListAds(ctx context.Context, userID, cursor int64, limit int) ([]AdWithSnapshots, int64, bool, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if cursor <= 0 {
		cursor = 1<<63 - 1
	}
	var ads []Ad
	if err := s.conn.QueryRowsCtx(ctx, &ads, `SELECT `+adColumns+` FROM ad WHERE user_id = ? AND id < ?
		ORDER BY id DESC LIMIT ?`, userID, cursor, limit+1); err != nil {
		return nil, 0, false, err
	}
	hasMore := len(ads) > limit
	if hasMore {
		ads = ads[:limit]
	}
	out := make([]AdWithSnapshots, 0, len(ads))
	for i := range ads {
		full, err := s.withSnapshots(ctx, &ads[i])
		if err != nil {
			return nil, 0, false, err
		}
		out = append(out, *full)
	}
	next := int64(0)
	if hasMore {
		next = ads[len(ads)-1].ID
	}
	return out, next, hasMore, nil
}

// withSnapshots 载入广告的最新快照与已过审快照（若有）。
func (s *Store) withSnapshots(ctx context.Context, ad *Ad) (*AdWithSnapshots, error) {
	out := &AdWithSnapshots{Ad: ad}
	latest, err := s.SnapshotAt(ctx, ad.ID, ad.Revision)
	if err != nil {
		return nil, err
	}
	out.Latest = latest
	if ad.ApprovedRevision > 0 {
		approved, err := s.SnapshotAt(ctx, ad.ID, ad.ApprovedRevision)
		if err != nil {
			return nil, err
		}
		out.Approved = approved
	}
	return out, nil
}

// SnapshotAt 读取指定 revision 的快照。
func (s *Store) SnapshotAt(ctx context.Context, adID, revision int64) (*Snapshot, error) {
	var snap Snapshot
	if err := s.conn.QueryRowCtx(ctx, &snap, `SELECT `+snapshotColumns+` FROM ad_snapshot WHERE ad_id = ? AND revision = ?`,
		adID, revision); err != nil {
		return nil, notFound(err)
	}
	return &snap, nil
}

// requireAsset 确认素材属于该用户且种类匹配；不存在与越权统一返回 ErrAssetInvalid。
func requireAsset(ctx context.Context, session sqlx.Session, userID, assetID int64, kind string) error {
	var owner struct {
		UserID int64  `db:"user_id"`
		Kind   string `db:"kind"`
	}
	err := session.QueryRowCtx(ctx, &owner, `SELECT user_id, kind FROM ad_asset WHERE id = ?`, assetID)
	if err != nil || owner.UserID != userID || owner.Kind != kind {
		if err != nil && !errors.Is(err, sqlx.ErrNotFound) {
			return err
		}
		return ErrAssetInvalid
	}
	return nil
}

// resolveIdempotency 在当前事务内解析幂等键，返回资源 ID 以及本次是否新建。
func resolveIdempotency(ctx context.Context, session sqlx.Session, scope string, userID int64, key string, recordID, resourceID int64, parts ...string) (int64, bool, error) {
	return resolve(ctx, session, scope, userID, key, recordID, resourceID, parts...)
}

// commandParts 列出参与幂等指纹的命令字段，同一幂等键携带不同内容时会被识别为冲突。
func commandParts(expectedRevision int64, in AdInput, extra ...string) []string {
	media := make([]string, len(in.MediaIDs))
	for i, id := range in.MediaIDs {
		media[i] = strconv.FormatInt(id, 10)
	}
	parts := []string{strconv.FormatInt(expectedRevision, 10), in.Title, in.Body, in.CTA, in.LandingURL,
		strings.Join(media, ","), in.Market, in.Industry, strconv.FormatInt(in.StartMs, 10), strconv.FormatInt(in.EndMs, 10)}
	return append(parts, extra...)
}
