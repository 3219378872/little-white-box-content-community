package model

import (
	"context"
	"errors"
)

var (
	ErrNotApplicable   = errors.New("recommend source is not applicable")
	ErrSnapshotMissing = errors.New("recommend snapshot is missing")
)

// RecallRequest 是一次召回的输入：身份、场景、请求元数据、规模与可选的种子帖。
type RecallRequest struct {
	UserID       int64
	AnonymousID  string
	Identity     string
	Scene        string
	RequestID    string
	SessionID    string
	ExperimentID string
	SeedPostID   int64
	Limit        int
}

// PostCandidate 是帖子候选，在召回、过滤、排序各阶段逐步补全。
type PostCandidate struct {
	PostID       int64
	RecallScore  float64
	RecallSource string
	Reason       string
	AuthorID     int64
	Category     string
	Features     PostFeatures
	CoarseScore  float64
	FinalScore   float64
	ModelVersion string
}

// UserCandidate 是用户候选，在召回、过滤、排序各阶段逐步补全。
type UserCandidate struct {
	UserID       int64
	RecallScore  float64
	RecallSource string
	Reason       string
	Category     string
	Features     UserFeatures
	FinalScore   float64
	ModelVersion string
}

// ViewerFeatures 是观看者的行为特征：正/负反馈、已曝光与已拉黑的作者。
type ViewerFeatures struct {
	PositivePostIDs map[int64]struct{}
	NegativePostIDs map[int64]struct{}
	SeenPostIDs     map[int64]struct{}
	BlockedAuthors  map[int64]struct{}
}

// PostFeatures 是帖子特征；Known 表示特征存在，Available 表示仍可推荐。
type PostFeatures struct {
	Known      bool
	Available  bool
	Visibility string
	AuthorID   int64
	Category   string
	Quality    float64
	CTR        float64
	Freshness  float64
	Popularity float64
}

// UserFeatures 是用户特征；Known 表示特征存在，Available 表示仍可推荐。
type UserFeatures struct {
	Known            bool
	Available        bool
	Visibility       string
	Category         string
	Quality          float64
	MutualCount      float64
	InterestAffinity float64
}

// RankedPost 是写入快照的一条排序结果。
type RankedPost struct {
	PostID       int64   `json:"post_id"`
	Score        float64 `json:"score"`
	Reason       string  `json:"reason"`
	RecallSource string  `json:"recall_source"`
	ModelVersion string  `json:"model_version"`
	ExperimentID string  `json:"experiment_id"`
	Position     int32   `json:"position"`
}

// PostSnapshot 是一次推荐的完整排序结果及其请求绑定，翻页时校验绑定后按偏移量续读。
type PostSnapshot struct {
	RuleOnly     bool         `json:"rule_only"`
	RequestID    string       `json:"request_id"`
	IdentityHash string       `json:"identity_hash"`
	Scene        string       `json:"scene"`
	SessionID    string       `json:"session_id"`
	ExperimentID string       `json:"experiment_id"`
	ExpiresAt    int64        `json:"expires_at"`
	Posts        []RankedPost `json:"posts"`
}

// InferenceResult 是精排服务返回的分数与模型版本。
type InferenceResult struct {
	Scores       map[int64]float64
	ModelVersion string
}

// PostRecallSource 是一路帖子召回。
type PostRecallSource interface {
	Name() string
	Recall(ctx context.Context, req RecallRequest) ([]PostCandidate, error)
}

// UserRecallSource 是一路用户召回。
type UserRecallSource interface {
	Name() string
	Recall(ctx context.Context, req RecallRequest) ([]UserCandidate, error)
}

// FeatureRepository 读取观看者、帖子与用户特征以及个性化开关。
type FeatureRepository interface {
	LoadViewerFeatures(ctx context.Context, identity string) (ViewerFeatures, error)
	LoadPostFeatures(ctx context.Context, postIDs []int64) (map[int64]PostFeatures, error)
	LoadUserFeatures(ctx context.Context, userIDs []int64) (map[int64]UserFeatures, error)
	// IsPersonalizationOptedOut 返回认证用户是否关闭了个性化（REL-023）。
	IsPersonalizationOptedOut(ctx context.Context, userID int64) (bool, error)
}

// SnapshotStore 保存与读取推荐快照。
type SnapshotStore interface {
	Save(ctx context.Context, snapshotID string, snapshot PostSnapshot, ttlSeconds int) error
	Load(ctx context.Context, snapshotID string) (PostSnapshot, error)
}

// InferenceRanker 是在线精排服务。
type InferenceRanker interface {
	Rank(ctx context.Context, requestID, modelVersion string, candidates []PostCandidate) (InferenceResult, error)
}
