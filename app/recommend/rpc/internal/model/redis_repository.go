package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type redisClient interface {
	GetCtx(ctx context.Context, key string) (string, error)
	HgetallCtx(ctx context.Context, key string) (map[string]string, error)
	LrangeCtx(ctx context.Context, key string, start, stop int) ([]string, error)
	SetexCtx(ctx context.Context, key, value string, seconds int) error
	SmembersCtx(ctx context.Context, key string) ([]string, error)
	ZrevrangeWithScoresByFloatCtx(ctx context.Context, key string, start, stop int64) ([]redis.FloatPair, error)
}

type RedisPostRecallSource struct {
	name       string
	reason     string
	redis      redisClient
	keyBuilder func(RecallRequest) string
}

func NewRedisPostRecallSource(name, reason string, redisClient redisClient, keyBuilder func(RecallRequest) string) *RedisPostRecallSource {
	return &RedisPostRecallSource{name: name, reason: reason, redis: redisClient, keyBuilder: keyBuilder}
}

func (s *RedisPostRecallSource) Name() string {
	return s.name
}

func (s *RedisPostRecallSource) Recall(ctx context.Context, req RecallRequest) ([]PostCandidate, error) {
	if req.Limit <= 0 {
		return nil, fmt.Errorf("%s post recall limit must be positive", s.name)
	}
	key := s.keyBuilder(req)
	if key == "" {
		return nil, ErrNotApplicable
	}
	pairs, err := s.redis.ZrevrangeWithScoresByFloatCtx(ctx, key, 0, int64(req.Limit-1))
	if err != nil {
		return nil, fmt.Errorf("load %s post recall: %w", s.name, err)
	}
	result := make([]PostCandidate, 0, len(pairs))
	for _, pair := range pairs {
		postID, err := strconv.ParseInt(pair.Key, 10, 64)
		if err != nil || postID <= 0 {
			return nil, fmt.Errorf("parse %s post candidate %q", s.name, pair.Key)
		}
		result = append(result, PostCandidate{
			PostID:       postID,
			RecallScore:  pair.Score,
			RecallSource: s.name,
			Reason:       s.reason,
		})
	}
	return result, nil
}

type RedisUserRecallSource struct {
	name       string
	reason     string
	redis      redisClient
	keyBuilder func(RecallRequest) string
}

func NewRedisUserRecallSource(name, reason string, redisClient redisClient, keyBuilder func(RecallRequest) string) *RedisUserRecallSource {
	return &RedisUserRecallSource{name: name, reason: reason, redis: redisClient, keyBuilder: keyBuilder}
}

func (s *RedisUserRecallSource) Name() string {
	return s.name
}

func (s *RedisUserRecallSource) Recall(ctx context.Context, req RecallRequest) ([]UserCandidate, error) {
	if req.Limit <= 0 {
		return nil, fmt.Errorf("%s user recall limit must be positive", s.name)
	}
	key := s.keyBuilder(req)
	if key == "" {
		return nil, ErrNotApplicable
	}
	pairs, err := s.redis.ZrevrangeWithScoresByFloatCtx(ctx, key, 0, int64(req.Limit-1))
	if err != nil {
		return nil, fmt.Errorf("load %s user recall: %w", s.name, err)
	}
	result := make([]UserCandidate, 0, len(pairs))
	for _, pair := range pairs {
		userID, err := strconv.ParseInt(pair.Key, 10, 64)
		if err != nil || userID <= 0 {
			return nil, fmt.Errorf("parse %s user candidate %q", s.name, pair.Key)
		}
		result = append(result, UserCandidate{
			UserID:       userID,
			RecallScore:  pair.Score,
			RecallSource: s.name,
			Reason:       s.reason,
		})
	}
	return result, nil
}

type RedisFeatureRepository struct {
	redis          redisClient
	featureVersion string
}

const featureLoadWorkers = 16

func NewRedisFeatureRepository(redisClient redisClient, featureVersion string) *RedisFeatureRepository {
	return &RedisFeatureRepository{redis: redisClient, featureVersion: featureVersion}
}

func (r *RedisFeatureRepository) LoadViewerFeatures(ctx context.Context, identity string) (ViewerFeatures, error) {
	features := ViewerFeatures{
		PositivePostIDs: make(map[int64]struct{}),
		NegativePostIDs: make(map[int64]struct{}),
		SeenPostIDs:     make(map[int64]struct{}),
		BlockedAuthors:  make(map[int64]struct{}),
	}
	if identity == "" {
		return features, nil
	}
	prefix := "feature:" + r.featureVersion + ":" + identity
	positive, err := r.redis.HgetallCtx(ctx, prefix+":positive")
	if err != nil {
		return ViewerFeatures{}, fmt.Errorf("load positive features: %w", err)
	}
	negative, err := r.redis.HgetallCtx(ctx, prefix+":negative")
	if err != nil {
		return ViewerFeatures{}, fmt.Errorf("load negative features: %w", err)
	}
	recent, err := r.redis.LrangeCtx(ctx, prefix+":recent", 0, 49)
	if err != nil {
		return ViewerFeatures{}, fmt.Errorf("load recent features: %w", err)
	}
	blockedAuthors, err := r.redis.SmembersCtx(ctx, prefix+":blocked_authors")
	if err != nil {
		return ViewerFeatures{}, fmt.Errorf("load blocked authors: %w", err)
	}
	if err := addTargetPostIDs(features.PositivePostIDs, positive); err != nil {
		return ViewerFeatures{}, fmt.Errorf("parse positive features: %w", err)
	}
	if err := addTargetPostIDs(features.NegativePostIDs, negative); err != nil {
		return ViewerFeatures{}, fmt.Errorf("parse negative features: %w", err)
	}
	for _, item := range recent {
		var event struct {
			Action     string `json:"action"`
			TargetID   int64  `json:"target_id"`
			TargetType string `json:"target_type"`
		}
		if err := json.Unmarshal([]byte(item), &event); err != nil {
			return ViewerFeatures{}, fmt.Errorf("parse recent feature: %w", err)
		}
		if event.TargetType == "post" && event.TargetID > 0 && event.Action == "exposure" {
			features.SeenPostIDs[event.TargetID] = struct{}{}
		}
	}
	for _, rawID := range blockedAuthors {
		authorID, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || authorID <= 0 {
			return ViewerFeatures{}, fmt.Errorf("parse blocked author %q", rawID)
		}
		features.BlockedAuthors[authorID] = struct{}{}
	}
	return features, nil
}

func (r *RedisFeatureRepository) LoadPostFeatures(ctx context.Context, postIDs []int64) (map[int64]PostFeatures, error) {
	result := make(map[int64]PostFeatures, len(postIDs))
	if len(postIDs) == 0 {
		return result, nil
	}
	type loadResult struct {
		postID   int64
		features PostFeatures
		err      error
	}
	jobs := make(chan int64, len(postIDs))
	results := make(chan loadResult, len(postIDs))
	for _, postID := range postIDs {
		jobs <- postID
	}
	close(jobs)
	workerCount := min(featureLoadWorkers, len(postIDs))
	var group sync.WaitGroup
	for range workerCount {
		group.Add(1)
		go func() {
			defer group.Done()
			for postID := range jobs {
				values, err := r.redis.HgetallCtx(ctx, fmt.Sprintf("feature:%s:post:%d", r.featureVersion, postID))
				if err != nil {
					results <- loadResult{postID: postID, err: fmt.Errorf("load post %d features: %w", postID, err)}
					continue
				}
				features, err := parsePostFeatures(values)
				if err != nil {
					results <- loadResult{postID: postID, err: fmt.Errorf("parse post %d features: %w", postID, err)}
					continue
				}
				results <- loadResult{postID: postID, features: features}
			}
		}()
	}
	go func() {
		group.Wait()
		close(results)
	}()
	var firstErr error
	for loaded := range results {
		if loaded.err != nil {
			if firstErr == nil {
				firstErr = loaded.err
			}
			continue
		}
		result[loaded.postID] = loaded.features
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return result, nil
}

func (r *RedisFeatureRepository) LoadUserFeatures(ctx context.Context, userIDs []int64) (map[int64]UserFeatures, error) {
	result := make(map[int64]UserFeatures, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	type loadResult struct {
		userID   int64
		features UserFeatures
		err      error
	}
	jobs := make(chan int64, len(userIDs))
	results := make(chan loadResult, len(userIDs))
	for _, userID := range userIDs {
		jobs <- userID
	}
	close(jobs)
	workerCount := min(featureLoadWorkers, len(userIDs))
	var group sync.WaitGroup
	for range workerCount {
		group.Add(1)
		go func() {
			defer group.Done()
			for userID := range jobs {
				values, err := r.redis.HgetallCtx(ctx, fmt.Sprintf("feature:%s:user:%d", r.featureVersion, userID))
				if err != nil {
					results <- loadResult{userID: userID, err: fmt.Errorf("load user %d features: %w", userID, err)}
					continue
				}
				features, err := parseUserFeatures(values)
				if err != nil {
					results <- loadResult{userID: userID, err: fmt.Errorf("parse user %d features: %w", userID, err)}
					continue
				}
				results <- loadResult{userID: userID, features: features}
			}
		}()
	}
	go func() {
		group.Wait()
		close(results)
	}()
	var firstErr error
	for loaded := range results {
		if loaded.err != nil {
			if firstErr == nil {
				firstErr = loaded.err
			}
			continue
		}
		result[loaded.userID] = loaded.features
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return result, nil
}

type RedisSnapshotStore struct {
	redis  redisClient
	prefix string
}

func NewRedisSnapshotStore(redisClient redisClient, prefix string) *RedisSnapshotStore {
	return &RedisSnapshotStore{redis: redisClient, prefix: prefix}
}

func (s *RedisSnapshotStore) Save(ctx context.Context, snapshotID string, snapshot PostSnapshot, ttlSeconds int) error {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal recommendation snapshot: %w", err)
	}
	if err := s.redis.SetexCtx(ctx, s.key(snapshotID), string(encoded), ttlSeconds); err != nil {
		return fmt.Errorf("save recommendation snapshot: %w", err)
	}
	return nil
}

func (s *RedisSnapshotStore) Load(ctx context.Context, snapshotID string) (PostSnapshot, error) {
	encoded, err := s.redis.GetCtx(ctx, s.key(snapshotID))
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return PostSnapshot{}, ErrSnapshotMissing
		}
		return PostSnapshot{}, fmt.Errorf("load recommendation snapshot: %w", err)
	}
	if encoded == "" {
		return PostSnapshot{}, ErrSnapshotMissing
	}
	var snapshot PostSnapshot
	if err := json.Unmarshal([]byte(encoded), &snapshot); err != nil {
		return PostSnapshot{}, fmt.Errorf("parse recommendation snapshot: %w", err)
	}
	return snapshot, nil
}

func (s *RedisSnapshotStore) key(snapshotID string) string {
	return s.prefix + ":snapshot:" + snapshotID
}

func IdentityKey(userID int64, anonymousID string) string {
	if userID > 0 {
		return "u:" + strconv.FormatInt(userID, 10)
	}
	if anonymousID == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(anonymousID))
	return "a:" + hex.EncodeToString(digest[:8])
}

func addTargetPostIDs(target map[int64]struct{}, values map[string]string) error {
	for field := range values {
		if !strings.HasPrefix(field, "post:") {
			continue
		}
		postID, err := strconv.ParseInt(strings.TrimPrefix(field, "post:"), 10, 64)
		if err != nil || postID <= 0 {
			return fmt.Errorf("invalid post target %q", field)
		}
		target[postID] = struct{}{}
	}
	return nil
}

func parsePostFeatures(values map[string]string) (PostFeatures, error) {
	features := PostFeatures{Known: len(values) > 0, Available: true, Visibility: "public"}
	status := strings.ToLower(values["status"])
	if status != "" && status != "published" && status != "active" {
		features.Available = false
	}
	if visibility := strings.ToLower(values["visibility"]); visibility != "" {
		features.Visibility = visibility
	}
	var err error
	if features.AuthorID, err = parseIntFeature(values, "author_id"); err != nil {
		return PostFeatures{}, err
	}
	features.Category = values["category"]
	if features.Quality, err = parseFloatFeature(values, "quality_score"); err != nil {
		return PostFeatures{}, err
	}
	if features.CTR, err = parseFloatFeature(values, "ctr"); err != nil {
		return PostFeatures{}, err
	}
	if features.Freshness, err = parseFloatFeature(values, "freshness"); err != nil {
		return PostFeatures{}, err
	}
	if features.Popularity, err = parseFloatFeature(values, "popularity"); err != nil {
		return PostFeatures{}, err
	}
	return features, nil
}

func parseUserFeatures(values map[string]string) (UserFeatures, error) {
	features := UserFeatures{Known: len(values) > 0, Available: true, Visibility: "public"}
	status := strings.ToLower(values["status"])
	if status != "" && status != "active" {
		features.Available = false
	}
	if visibility := strings.ToLower(values["visibility"]); visibility != "" {
		features.Visibility = visibility
	}
	features.Category = values["category"]
	var err error
	if features.Quality, err = parseFloatFeature(values, "quality_score"); err != nil {
		return UserFeatures{}, err
	}
	if features.MutualCount, err = parseFloatFeature(values, "mutual_count"); err != nil {
		return UserFeatures{}, err
	}
	if features.InterestAffinity, err = parseFloatFeature(values, "interest_affinity"); err != nil {
		return UserFeatures{}, err
	}
	return features, nil
}

func parseFloatFeature(values map[string]string, field string) (float64, error) {
	raw := values[field]
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q", field, raw)
	}
	return value, nil
}

func parseIntFeature(values map[string]string, field string) (int64, error) {
	raw := values[field]
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q", field, raw)
	}
	return value, nil
}
