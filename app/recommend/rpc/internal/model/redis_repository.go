package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"esx/app/recommend/featurekey"
	"esx/app/recommend/personalization"
	"esx/app/user/rpc/userservice"

	redis "esx/pkg/redisstore"
)

// redisClient 是推荐读路径用到的 Redis 子集，便于测试替换。
type redisClient interface {
	GetCtx(ctx context.Context, key string) (string, error)
	HgetallCtx(ctx context.Context, key string) (map[string]string, error)
	LrangeCtx(ctx context.Context, key string, start, stop int) ([]string, error)
	SetexCtx(ctx context.Context, key, value string, seconds int) error
	SmembersCtx(ctx context.Context, key string) ([]string, error)
	ZrevrangeWithScoresByFloatCtx(ctx context.Context, key string, start, stop int64) ([]redis.FloatPair, error)
}

// RedisPostRecallSource 从 Redis 有序集合召回帖子；keyBuilder 返回空键表示该请求不适用此来源。
type RedisPostRecallSource struct {
	name       string
	reason     string
	redis      redisClient
	keyBuilder func(RecallRequest) string
}

// NewRedisPostRecallSource 创建从 Redis 有序集合召回帖子的来源；keyBuilder 决定读哪个集合。
func NewRedisPostRecallSource(name, reason string, redisClient redisClient, keyBuilder func(RecallRequest) string) *RedisPostRecallSource {
	return &RedisPostRecallSource{name: name, reason: reason, redis: redisClient, keyBuilder: keyBuilder}
}

// Name 返回召回来源标识。
func (s *RedisPostRecallSource) Name() string {
	return s.name
}

// Recall 按分数从高到低取前 Limit 个帖子，并标注召回来源与推荐理由。
func (s *RedisPostRecallSource) Recall(ctx context.Context, req RecallRequest) ([]PostCandidate, error) {
	ranked, err := recallRanked(ctx, s.redis, s.keyBuilder(req), req.Limit, s.name, "post")
	if err != nil {
		return nil, err
	}
	result := make([]PostCandidate, 0, len(ranked))
	for _, item := range ranked {
		result = append(result, PostCandidate{
			PostID:       item.id,
			RecallScore:  item.score,
			RecallSource: s.name,
			Reason:       s.reason,
		})
	}
	return result, nil
}

// RedisUserRecallSource 是用户推荐版本的有序集合召回源。
type RedisUserRecallSource struct {
	name       string
	reason     string
	redis      redisClient
	keyBuilder func(RecallRequest) string
}

// NewRedisUserRecallSource 创建从 Redis 有序集合召回用户的来源。
func NewRedisUserRecallSource(name, reason string, redisClient redisClient, keyBuilder func(RecallRequest) string) *RedisUserRecallSource {
	return &RedisUserRecallSource{name: name, reason: reason, redis: redisClient, keyBuilder: keyBuilder}
}

// Name 返回召回来源标识。
func (s *RedisUserRecallSource) Name() string {
	return s.name
}

// Recall 按分数从高到低取前 Limit 个用户，并标注召回来源与推荐理由。
func (s *RedisUserRecallSource) Recall(ctx context.Context, req RecallRequest) ([]UserCandidate, error) {
	ranked, err := recallRanked(ctx, s.redis, s.keyBuilder(req), req.Limit, s.name, "user")
	if err != nil {
		return nil, err
	}
	result := make([]UserCandidate, 0, len(ranked))
	for _, item := range ranked {
		result = append(result, UserCandidate{
			UserID:       item.id,
			RecallScore:  item.score,
			RecallSource: s.name,
			Reason:       s.reason,
		})
	}
	return result, nil
}

// rankedID 是召回有序集合中的一个成员及其分数。
type rankedID struct {
	id    int64
	score float64
}

// recallRanked 是帖子与用户召回源共用的读取步骤：校验 limit、读取有序集合前 limit 名并解析正整数 ID。
// kind 只用于错误信息，区分 post / user 召回。
func recallRanked(ctx context.Context, client redisClient, key string, limit int, source, kind string) ([]rankedID, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("%s %s recall limit must be positive", source, kind)
	}
	// 空键说明请求缺少该来源所需的身份或种子，交由上层跳过此来源。
	if key == "" {
		return nil, ErrNotApplicable
	}
	pairs, err := client.ZrevrangeWithScoresByFloatCtx(ctx, key, 0, int64(limit-1))
	if err != nil {
		return nil, fmt.Errorf("load %s %s recall: %w", source, kind, err)
	}
	result := make([]rankedID, 0, len(pairs))
	for _, pair := range pairs {
		// 成员必须是正整数 ID；脏数据直接报错，不悄悄丢弃。
		id, err := strconv.ParseInt(pair.Key, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("parse %s %s candidate %q", source, kind, pair.Key)
		}
		result = append(result, rankedID{id: id, score: pair.Score})
	}
	return result, nil
}

// RedisFeatureRepository 读取 recommend-mq 写入的在线特征，并复核个性化开关。
type RedisFeatureRepository struct {
	preferences personalization.PreferenceReader
	redis       redisClient
	features    featurekey.Space
	now         func() time.Time
}

const featureLoadWorkers = 16

// NewRedisFeatureRepository 绑定特征版本；readers 为空且无关闭标记时，个性化检查报错并按已关闭处理（失败即保守）。
func NewRedisFeatureRepository(redisClient redisClient, featureVersion string, readers ...personalization.PreferenceReader) *RedisFeatureRepository {
	r := &RedisFeatureRepository{redis: redisClient, features: featurekey.New(featureVersion)}
	if len(readers) > 0 {
		r.preferences = readers[0]
	}
	return r
}

// LoadViewerFeatures 读取观看者的正/负反馈、近期曝光与屏蔽作者，供召回过滤与排序使用；
// 空身份返回空特征而不是错误。
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
	// 依次读取观看者的四类特征键，任一失败都整体失败，不用残缺特征排序。
	prefix := r.features.Viewer(identity)
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
	// 把原始哈希、列表与集合解析成按 ID 查询的集合。
	if err := addTargetPostIDs(features.PositivePostIDs, positive); err != nil {
		return ViewerFeatures{}, fmt.Errorf("parse positive features: %w", err)
	}
	if err := addTargetPostIDs(features.NegativePostIDs, negative); err != nil {
		return ViewerFeatures{}, fmt.Errorf("parse negative features: %w", err)
	}
	// DISC-035：已曝光内容只排除最近 7 天；更早的曝光不作为排除依据。
	seenWindow := time.Now().Add(-7 * 24 * time.Hour).UnixMilli()
	if r.now != nil {
		seenWindow = r.now().Add(-7 * 24 * time.Hour).UnixMilli()
	}
	for _, item := range recent {
		var event struct {
			Action     string `json:"action"`
			TargetID   int64  `json:"target_id"`
			TargetType string `json:"target_type"`
			EventTime  int64  `json:"event_time"`
		}
		if err := json.Unmarshal([]byte(item), &event); err != nil {
			return ViewerFeatures{}, fmt.Errorf("parse recent feature: %w", err)
		}
		if event.TargetType == "post" && event.TargetID > 0 && event.Action == "exposure" &&
			event.EventTime >= seenWindow {
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

// LoadPostFeatures 并发读取候选帖子的特征哈希，供排序打分使用。
func (r *RedisFeatureRepository) LoadPostFeatures(ctx context.Context, postIDs []int64) (map[int64]PostFeatures, error) {
	return loadFeatures(postIDs, func(postID int64) (PostFeatures, error) {
		values, err := r.redis.HgetallCtx(ctx, r.features.Post(postID))
		if err != nil {
			return PostFeatures{}, fmt.Errorf("load post %d features: %w", postID, err)
		}
		features, err := parsePostFeatures(values)
		if err != nil {
			return PostFeatures{}, fmt.Errorf("parse post %d features: %w", postID, err)
		}
		return features, nil
	})
}

// LoadUserFeatures 并发读取候选用户（作者）的特征哈希，供用户推荐排序使用。
func (r *RedisFeatureRepository) LoadUserFeatures(ctx context.Context, userIDs []int64) (map[int64]UserFeatures, error) {
	return loadFeatures(userIDs, func(userID int64) (UserFeatures, error) {
		values, err := r.redis.HgetallCtx(ctx, r.features.User(userID))
		if err != nil {
			return UserFeatures{}, fmt.Errorf("load user %d features: %w", userID, err)
		}
		features, err := parseUserFeatures(values)
		if err != nil {
			return UserFeatures{}, fmt.Errorf("parse user %d features: %w", userID, err)
		}
		return features, nil
	})
}

// loadFeatures 用最多 featureLoadWorkers 个协程逐个加载 ID 的特征；
// 候选数通常上百，串行往返 Redis 会拖慢整次推荐。任一加载失败即整体失败，
// 返回首个收到的错误，不返回部分结果，避免排序在缺特征时静默降级。
func loadFeatures[T any](ids []int64, load func(int64) (T, error)) (map[int64]T, error) {
	result := make(map[int64]T, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	type loadResult struct {
		id       int64
		features T
		err      error
	}
	// 任务与结果通道都按 ID 数预留容量，worker 不会因收集方阻塞。
	jobs := make(chan int64, len(ids))
	results := make(chan loadResult, len(ids))
	for _, id := range ids {
		jobs <- id
	}
	close(jobs)
	workerCount := min(featureLoadWorkers, len(ids))
	var group sync.WaitGroup
	for range workerCount {
		group.Go(func() {
			for id := range jobs {
				features, err := load(id)
				results <- loadResult{id: id, features: features, err: err}
			}
		})
	}
	go func() {
		group.Wait()
		close(results)
	}()
	// 收集时排空全部结果再返回，确保 worker 协程都已退出。
	var firstErr error
	for loaded := range results {
		if loaded.err != nil {
			if firstErr == nil {
				firstErr = loaded.err
			}
			continue
		}
		result[loaded.id] = loaded.features
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return result, nil
}

// IsPersonalizationOptedOut 检查用户是否关闭了个性化（REL-023）。
// 标记由 user 服务在关闭时写入 `personalization:optout:<userID>`。
func (r *RedisFeatureRepository) IsPersonalizationOptedOut(ctx context.Context, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	// A missing/expired marker is never proof of consent. A cache outage also
	// falls back to the user service, which owns the durable preference.
	value, err := r.redis.GetCtx(ctx, featurekey.OptOutKey(userID))
	if err == nil && value != "" {
		return true, nil
	}
	if r.preferences == nil {
		return true, fmt.Errorf("personalization preference service unavailable")
	}
	preference, err := r.preferences.GetPersonalizationPreference(ctx, &userservice.GetPersonalizationPreferenceReq{UserId: userID})
	if err != nil {
		return true, fmt.Errorf("load personalization preference: %w", err)
	}
	if preference == nil {
		return true, fmt.Errorf("personalization preference response is nil")
	}
	return !preference.Enabled, nil
}

// RedisSnapshotStore 把一次推荐的排序结果存为快照，翻页时从快照续读，保证页间不重复不遗漏。
type RedisSnapshotStore struct {
	redis  redisClient
	prefix string
}

// NewRedisSnapshotStore 创建快照存储。
func NewRedisSnapshotStore(redisClient redisClient, prefix string) *RedisSnapshotStore {
	return &RedisSnapshotStore{redis: redisClient, prefix: prefix}
}

// Save 保存快照并设置 TTL。
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

// Load 读取快照；过期或不存在时返回 ErrSnapshotMissing。
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

// key 是快照的 Redis 键。
func (s *RedisSnapshotStore) key(snapshotID string) string {
	return s.prefix + ":snapshot:" + snapshotID
}

// addTargetPostIDs 从行为哈希中提取 post:{id} 字段对应的帖子 ID。
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

// parsePostFeatures 解析帖子特征哈希；状态非发布即视为不可用，缺省可见性为公开。
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

// parseUserFeatures 解析用户特征哈希；状态非 active 即视为不可用，缺省可见性为公开。
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

// parseFloatFeature 解析浮点特征；缺失为 0，格式错误返回错误。
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

// parseIntFeature 解析整数特征；缺失为 0，格式错误返回错误。
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
