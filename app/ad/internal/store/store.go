// Package store 是付费广告的权威存储（xbh_ad，SPEC-sponsored-ads）。
//
// 广告与广告主的写操作带 expectedRevision 与幂等键（ADS-013），并在同一事务内写快照与
// review-submitted 送审事件（ADS-011）。审核结论只经 review-decided 事件按 revision CAS 应用；
// 本包不提供把对象直接置为通过的写入口（RVW-051）。
package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"esx/pkg/outboxx"
	sqlx "esx/pkg/sqlstore"
	"esx/pkg/util"
)

// 审核状态（ADS-010）。
const (
	ReviewDraft     = "draft"
	ReviewPending   = "pending_review"
	ReviewApproved  = "approved"
	ReviewRejected  = "rejected"
	ReviewAppealing = "appealing"
)

// 投放状态。
const (
	ServingNone    = "none"
	ServingServing = "serving"
	ServingPaused  = "paused"
	ServingOffline = "offline"
)

// 资质状态。
const (
	QualificationPending  = "pending"
	QualificationApproved = "approved"
	QualificationRejected = "rejected"
	QualificationExpired  = "expired"
)

// 资产类型。
const (
	AssetCreative = "creative"
	AssetDocument = "document"
)

var (
	ErrNotFound              = errors.New("ad: not found")
	ErrRevisionConflict      = errors.New("ad: revision conflict")
	ErrAdvertiserExists      = errors.New("ad: advertiser already exists")
	ErrAdvertiserNotApproved = errors.New("ad: advertiser not approved")
	ErrQualificationMissing  = errors.New("ad: qualification missing")
	ErrAssetInvalid          = errors.New("ad: asset invalid")
	ErrMarketNotAllowed      = errors.New("ad: market not allowed for advertiser")
)

// Advertiser 是 advertiser 一行。
type Advertiser struct {
	ID               int64  `db:"id"`
	UserID           int64  `db:"user_id"`
	Name             string `db:"name"`
	MarketsCSV       string `db:"markets"`
	Revision         int64  `db:"revision"`
	ApprovedRevision int64  `db:"approved_revision"`
	ReviewStatus     string `db:"review_status"`
	PolicyCodesJSON  string `db:"policy_codes"`
	ApprovedName     string `db:"approved_name"`
	ReviewTaskID     int64  `db:"review_task_id"`
	SubmittedAtMs    int64  `db:"submitted_at_ms"`
	CreatedAtMs      int64  `db:"created_at_ms"`
	UpdatedAtMs      int64  `db:"updated_at_ms"`
}

const advertiserColumns = `id, user_id, name, markets, revision, approved_revision, review_status, policy_codes,
	approved_name, review_task_id, submitted_at_ms, created_at_ms, updated_at_ms`

func (a Advertiser) Markets() []string     { return splitCSV(a.MarketsCSV) }
func (a Advertiser) PolicyCodes() []string { return decodeCodes(a.PolicyCodesJSON) }

// Qualification 是 advertiser_qualification 一行。
type Qualification struct {
	ID                int64  `db:"id"`
	AdvertiserID      int64  `db:"advertiser_id"`
	Market            string `db:"market"`
	Industry          string `db:"industry"`
	DocumentMediaID   int64  `db:"document_media_id"`
	ValidUntilMs      int64  `db:"valid_until_ms"`
	Status            string `db:"status"`
	SubmittedRevision int64  `db:"submitted_revision"`
	CreatedAtMs       int64  `db:"created_at_ms"`
	UpdatedAtMs       int64  `db:"updated_at_ms"`
}

const qualificationColumns = `id, advertiser_id, market, industry, document_media_id, valid_until_ms, status,
	submitted_revision, created_at_ms, updated_at_ms`

// Ad 是 ad 一行。
type Ad struct {
	ID                int64  `db:"id"`
	AdvertiserID      int64  `db:"advertiser_id"`
	UserID            int64  `db:"user_id"`
	Revision          int64  `db:"revision"`
	ApprovedRevision  int64  `db:"approved_revision"`
	ReviewStatus      string `db:"review_status"`
	ServingStatus     string `db:"serving_status"`
	Market            string `db:"market"`
	Industry          string `db:"industry"`
	StartMs           int64  `db:"start_ms"`
	EndMs             int64  `db:"end_ms"`
	PolicyCodesJSON   string `db:"policy_codes"`
	PauseReason       string `db:"pause_reason"`
	AppealedRevision  int64  `db:"appealed_revision"`
	ReviewTaskID      int64  `db:"review_task_id"`
	SubmittedAtMs     int64  `db:"submitted_at_ms"`
	PublishedRevision int64  `db:"published_revision"`
	CreatedAtMs       int64  `db:"created_at_ms"`
	UpdatedAtMs       int64  `db:"updated_at_ms"`
}

const adColumns = `id, advertiser_id, user_id, revision, approved_revision, review_status, serving_status, market,
	industry, start_ms, end_ms, policy_codes, pause_reason, appealed_revision, review_task_id, submitted_at_ms,
	published_revision, created_at_ms, updated_at_ms`

func (a Ad) PolicyCodes() []string { return decodeCodes(a.PolicyCodesJSON) }

// SnapshotMedia 是快照中的一个素材。
type SnapshotMedia struct {
	MediaID int64  `json:"mediaId"`
	SHA256  string `json:"sha256"`
}

// Snapshot 是 ad_snapshot 一行（ADS-012：投放内容只来自过审快照）。
type Snapshot struct {
	AdID           int64  `db:"ad_id"`
	Revision       int64  `db:"revision"`
	AdvertiserName string `db:"advertiser_name"`
	Title          string `db:"title"`
	Body           string `db:"body"`
	CTA            string `db:"cta"`
	LandingURL     string `db:"landing_url"`
	LandingDomain  string `db:"landing_domain"`
	MediaJSON      string `db:"media"`
	Market         string `db:"market"`
	Language       string `db:"language"`
	Industry       string `db:"industry"`
	CreatedAtMs    int64  `db:"created_at_ms"`
}

const snapshotColumns = `ad_id, revision, advertiser_name, title, body, cta, landing_url, landing_domain, media,
	market, language, industry, created_at_ms`

// Media 解码快照素材。
func (s Snapshot) Media() []SnapshotMedia {
	var media []SnapshotMedia
	if err := json.Unmarshal([]byte(s.MediaJSON), &media); err != nil {
		return nil
	}
	return media
}

// Asset 是 ad_asset 一行。
type Asset struct {
	ID          int64  `db:"id"`
	UserID      int64  `db:"user_id"`
	Kind        string `db:"kind"`
	SHA256      string `db:"sha256"`
	MimeType    string `db:"mime_type"`
	SizeBytes   int64  `db:"size_bytes"`
	ObjectKey   string `db:"object_key"`
	CreatedAtMs int64  `db:"created_at_ms"`
}

const assetColumns = `id, user_id, kind, sha256, mime_type, size_bytes, object_key, created_at_ms`

// Store 封装 xbh_ad 读写。
type Store struct {
	conn   sqlx.SqlConn
	outbox *outboxx.SQLStore
	nextID func() (int64, error)
}

func New(conn sqlx.SqlConn) *Store {
	return &Store{conn: conn, outbox: outboxx.NewSQLStore(conn), nextID: util.NextID}
}

// Conn 暴露连接给需要只读查询的组件（投放索引重建）。
func (s *Store) Conn() sqlx.SqlConn { return s.conn }

func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func decodeCodes(raw string) []string {
	var codes []string
	if err := json.Unmarshal([]byte(raw), &codes); err != nil {
		return nil
	}
	return codes
}

func encodeCodes(codes []string) string {
	if codes == nil {
		codes = []string{}
	}
	raw, _ := json.Marshal(codes)
	return string(raw)
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func notFound(err error) error {
	if errors.Is(err, sqlx.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

type rowQuerier interface {
	QueryRowCtx(ctx context.Context, v any, query string, args ...any) error
	QueryRowsCtx(ctx context.Context, v any, query string, args ...any) error
}
