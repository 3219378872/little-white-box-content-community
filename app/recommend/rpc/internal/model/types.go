package model

import (
	"context"
	"errors"
)

var (
	ErrNotApplicable   = errors.New("recommend source is not applicable")
	ErrSnapshotMissing = errors.New("recommend snapshot is missing")
)

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

type ViewerFeatures struct {
	PositivePostIDs map[int64]struct{}
	NegativePostIDs map[int64]struct{}
	SeenPostIDs     map[int64]struct{}
	BlockedAuthors  map[int64]struct{}
}

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

type UserFeatures struct {
	Known            bool
	Available        bool
	Visibility       string
	Category         string
	Quality          float64
	MutualCount      float64
	InterestAffinity float64
}

type RankedPost struct {
	PostID       int64   `json:"post_id"`
	Score        float64 `json:"score"`
	Reason       string  `json:"reason"`
	RecallSource string  `json:"recall_source"`
	ModelVersion string  `json:"model_version"`
	ExperimentID string  `json:"experiment_id"`
	Position     int32   `json:"position"`
}

type PostSnapshot struct {
	RequestID    string       `json:"request_id"`
	IdentityHash string       `json:"identity_hash"`
	Scene        string       `json:"scene"`
	SessionID    string       `json:"session_id"`
	ExperimentID string       `json:"experiment_id"`
	ExpiresAt    int64        `json:"expires_at"`
	Posts        []RankedPost `json:"posts"`
}

type InferenceResult struct {
	Scores       map[int64]float64
	ModelVersion string
}

type PostRecallSource interface {
	Name() string
	Recall(ctx context.Context, req RecallRequest) ([]PostCandidate, error)
}

type UserRecallSource interface {
	Name() string
	Recall(ctx context.Context, req RecallRequest) ([]UserCandidate, error)
}

type FeatureRepository interface {
	LoadViewerFeatures(ctx context.Context, identity string) (ViewerFeatures, error)
	LoadPostFeatures(ctx context.Context, postIDs []int64) (map[int64]PostFeatures, error)
	LoadUserFeatures(ctx context.Context, userIDs []int64) (map[int64]UserFeatures, error)
	// IsPersonalizationOptedOut 返回认证用户是否关闭了个性化（REL-023）。
	IsPersonalizationOptedOut(ctx context.Context, userID int64) (bool, error)
}

type SnapshotStore interface {
	Save(ctx context.Context, snapshotID string, snapshot PostSnapshot, ttlSeconds int) error
	Load(ctx context.Context, snapshotID string) (PostSnapshot, error)
}

type InferenceRanker interface {
	Rank(ctx context.Context, requestID, modelVersion string, candidates []PostCandidate) (InferenceResult, error)
}
