package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"esx/pkg/event"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type BehaviorStore interface {
	Record(ctx context.Context, behavior event.BehaviorEvent) error
	// PurgeOptedOutFeatures 主动删除已关闭个性化用户的在线特征（REL-023），
	// 返回清理的用户数；不支持键枚举的存储返回 0,nil。
	PurgeOptedOutFeatures(ctx context.Context) (int, error)
}

type RedisEvaler interface {
	EvalCtx(ctx context.Context, script string, keys []string, args ...any) (any, error)
}

type RedisBehaviorStore struct {
	redis          RedisEvaler
	featureVersion string
	recallPrefix   string
	ttlSeconds     int
}

// personalizationOptOutKeyPrefix 与 user 服务写入的关闭标记保持一致（REL-023）。
const personalizationOptOutKeyPrefix = "personalization:optout:"

type RedisGetter interface {
	GetCtx(ctx context.Context, key string) (string, error)
}

func NewRedisBehaviorStore(redis RedisEvaler, featureVersion, recallKeyPrefix string, ttlSeconds int) *RedisBehaviorStore {
	return &RedisBehaviorStore{
		redis: redis, featureVersion: featureVersion,
		recallPrefix: recallKeyPrefix + ":" + featureVersion, ttlSeconds: ttlSeconds,
	}
}

func (s *RedisBehaviorStore) Record(ctx context.Context, behavior event.BehaviorEvent) error {
	if err := behavior.Validate(); err != nil {
		return fmt.Errorf("validate behavior feature event: %w", err)
	}
	identity := behaviorIdentity(behavior)
	if identity == "" {
		return fmt.Errorf("behavior feature identity is required")
	}
	// DISC-031：匿名行为不得建立跨会话匿名画像。匿名事件仍会写入行为分析
	// （ClickHouse），但不进入推荐在线特征，匿名推荐只使用非持久化冷启动来源。
	if behavior.UserID <= 0 {
		return nil
	}
	optedOut, err := s.personalizationOptedOut(ctx, behavior.UserID)
	if err != nil {
		return fmt.Errorf("check personalization opt-out: %w", err)
	}
	if optedOut {
		// REL-023：关闭个性化后立即停止新行为用于个性化，并清理在线特征。
		if err := s.purgeIdentityFeatures(ctx, identity); err != nil {
			return fmt.Errorf("purge opted-out features: %w", err)
		}
		return nil
	}
	prefix := "feature:" + s.featureVersion + ":" + identity
	targetID := strconv.FormatInt(behavior.TargetID, 10)
	scene := behavior.Scene
	if scene == "" {
		scene = "home"
	}
	recent, err := json.Marshal(map[string]any{
		"event_id": behavior.EventID, "client_event_id": behavior.ClientEventID,
		"request_id": behavior.RequestID, "action": behavior.Action,
		"target_id": behavior.TargetID, "target_type": behavior.TargetType,
		"scene": behavior.Scene, "event_time": behavior.EventTime,
	})
	if err != nil {
		return fmt.Errorf("marshal recent behavior: %w", err)
	}
	// REL-004：同一 (requestId, postId) 最多记录一次曝光。给曝光事件附加
	// 独立的去重键，避免客户端用不同 event id 重复上报同一曝光。
	exposureDedupKey := ""
	if behavior.Action == event.BehaviorActionExposure && strings.TrimSpace(behavior.RequestID) != "" {
		exposureDedupKey = prefix + ":exposure:dedup:" + behavior.RequestID + ":" + targetID
	}
	_, err = s.redis.EvalCtx(ctx, recordFeatureScript, []string{
		"feature:" + s.featureVersion + ":dedup:" + behavior.EventIDString(),
		exposureDedupKey,
		prefix + ":recent", prefix + ":positive", prefix + ":negative", prefix + ":scene",
		s.recallPrefix + ":recall:post:hot:" + scene,
		s.recallPrefix + ":recall:post:itemcf:" + identity + ":" + scene,
		s.recallPrefix + ":recall:post:itemcf:seed:" + targetID + ":" + scene,
		s.recallPrefix + ":recall:user:popular:" + scene,
		"feature:" + s.featureVersion + ":user:" + targetID,
		s.recallPrefix + ":follow:author:" + targetID + ":followers",
		s.recallPrefix + ":author:" + targetID + ":posts",
		s.recallPrefix + ":recall:post:follow:" + identity + ":" + scene,
		"feature:" + s.featureVersion + ":post:" + targetID,
		prefix + ":state",
	}, s.ttlSeconds, string(recent), behavior.Action,
		behavior.TargetType+":"+targetID, scene, targetID, behavior.TargetType,
		identity, s.recallPrefix, behavior.EventTime, behavior.ClientEventID)
	if err != nil {
		return fmt.Errorf("record behavior features: %w", err)
	}
	return nil
}

func (s *RedisBehaviorStore) personalizationOptedOut(ctx context.Context, userID int64) (bool, error) {
	getter, ok := s.redis.(RedisGetter)
	if !ok {
		return false, nil
	}
	value, err := getter.GetCtx(ctx, personalizationOptOutKeyPrefix+strconv.FormatInt(userID, 10))
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, err
	}
	return value != "", nil
}

// purgeIdentityFeatures 删除该身份的全部在线个性化特征与个性化召回键。
func (s *RedisBehaviorStore) purgeIdentityFeatures(ctx context.Context, identity string) error {
	prefix := "feature:" + s.featureVersion + ":" + identity
	purger, ok := s.redis.(interface {
		EvalCtx(ctx context.Context, script string, keys []string, args ...any) (any, error)
	})
	if !ok {
		return nil
	}
	_, err := purger.EvalCtx(ctx, purgeIdentityFeaturesScript, []string{
		prefix + ":recent", prefix + ":positive", prefix + ":negative", prefix + ":scene",
		prefix + ":state", prefix + ":blocked_authors",
		s.recallPrefix + ":recall:post:itemcf:" + identity,
		s.recallPrefix + ":recall:post:follow:" + identity,
		s.recallPrefix + ":recall:user:interest:" + identity,
		s.recallPrefix + ":recall:user:mutual:" + identity,
	}, 0)
	return err
}

const purgeIdentityFeaturesScript = `
for index = 1, #KEYS do
  redis.call('DEL', KEYS[index])
end
return 1
`

func behaviorIdentity(behavior event.BehaviorEvent) string {
	if behavior.UserID > 0 {
		return "u:" + strconv.FormatInt(behavior.UserID, 10)
	}
	if behavior.AnonymousID == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(behavior.AnonymousID))
	return "a:" + hex.EncodeToString(digest[:8])
}

const recordFeatureScript = `
if redis.call('EXISTS', KEYS[1]) == 1 then
  return 0
end
redis.call('SETEX', KEYS[1], ARGV[1], '1')
-- REL-004：同一 (requestId, postId) 最多记录一次曝光；KEYS[2] 为曝光去重键，
-- 非曝光事件为空字符串且不会在此分支被引用。
local action = ARGV[3]
if action == 'exposure' and KEYS[2] ~= '' then
  if redis.call('EXISTS', KEYS[2]) == 1 then
    return 0
  end
  redis.call('SETEX', KEYS[2], ARGV[1], '1')
end
local recent = redis.call('LRANGE', KEYS[3], 0, 49)
table.insert(recent, ARGV[2])
table.sort(recent, function(left, right)
  local left_event = cjson.decode(left)
  local right_event = cjson.decode(right)
  local left_time = tonumber(left_event.event_time) or 0
  local right_time = tonumber(right_event.event_time) or 0
  if left_time == right_time then
    return tostring(left_event.client_event_id or '') > tostring(right_event.client_event_id or '')
  end
  return left_time > right_time
end)
redis.call('DEL', KEYS[3])
for index = 1, math.min(#recent, 50) do
  redis.call('RPUSH', KEYS[3], recent[index])
end
redis.call('EXPIRE', KEYS[3], ARGV[1])

local target = ARGV[4]
local scene = ARGV[5]
local target_id = ARGV[6]
local target_type = ARGV[7]
local identity = ARGV[8]
local recall_prefix = ARGV[9]
local event_time = tonumber(ARGV[10]) or 0
local client_event_id = ARGV[11]
local prior_positive = redis.call('HKEYS', KEYS[4])
if action == 'like' or action == 'favorite' or action == 'comment' or action == 'share' or action == 'click' or action == 'dwell' then
  redis.call('HINCRBY', KEYS[4], target, 1)
  redis.call('EXPIRE', KEYS[4], ARGV[1])
elseif action == 'unlike' or action == 'unfavorite' or action == 'hide' or action == 'dislike' then
  redis.call('HINCRBY', KEYS[5], target, 1)
  redis.call('EXPIRE', KEYS[5], ARGV[1])
end

if scene ~= '' then
  redis.call('HINCRBY', KEYS[6], scene, 1)
  redis.call('EXPIRE', KEYS[6], ARGV[1])
end

if target_type == 'post' then
  local weight = 0
  if action == 'exposure' then weight = 1
  elseif action == 'click' or action == 'dwell' then weight = 2
  elseif action == 'like' or action == 'favorite' or action == 'comment' or action == 'share' then weight = 5
  elseif action == 'hide' or action == 'dislike' then weight = -5
  end
  if weight ~= 0 then
    redis.call('ZINCRBY', KEYS[7], weight, target_id)
    redis.call('EXPIRE', KEYS[7], ARGV[1])
  end

  if action == 'like' or action == 'favorite' or action == 'comment' or action == 'share' or action == 'click' or action == 'dwell' then
    local neighbors = redis.call('ZREVRANGE', KEYS[9], 0, 99, 'WITHSCORES')
    for i = 1, #neighbors, 2 do
      if neighbors[i] ~= target_id then
        redis.call('ZINCRBY', KEYS[8], tonumber(neighbors[i + 1]), neighbors[i])
      end
    end
    for _, previous in ipairs(prior_positive) do
      local previous_id = string.match(previous, '^post:(%d+)$')
      if previous_id and previous_id ~= target_id then
        redis.call('ZINCRBY', KEYS[9], 1, previous_id)
        local reverse_key = recall_prefix .. ':recall:post:itemcf:seed:' .. previous_id .. ':' .. scene
        redis.call('ZINCRBY', reverse_key, 1, target_id)
        redis.call('EXPIRE', reverse_key, ARGV[1])
      end
    end
    redis.call('EXPIRE', KEYS[8], ARGV[1])
    redis.call('EXPIRE', KEYS[9], ARGV[1])
  end

  if redis.call('EXISTS', KEYS[15]) == 1 then
    if action == 'exposure' then redis.call('HINCRBY', KEYS[15], 'impressions', 1) end
    if action == 'click' then redis.call('HINCRBY', KEYS[15], 'clicks', 1) end
    redis.call('HINCRBY', KEYS[15], 'popularity', weight)
    local impressions = tonumber(redis.call('HGET', KEYS[15], 'impressions') or '0')
    local clicks = tonumber(redis.call('HGET', KEYS[15], 'clicks') or '0')
    if impressions > 0 then redis.call('HSET', KEYS[15], 'ctr', clicks / impressions) end
  end
elseif target_type == 'user' and (action == 'follow' or action == 'unfollow') then
  local delta = 1
  if action == 'unfollow' then delta = -1 end
  redis.call('ZINCRBY', KEYS[10], delta, target_id)
  redis.call('EXPIRE', KEYS[10], ARGV[1])
  redis.call('HSET', KEYS[11], 'status', 'active', 'visibility', 'public')
  redis.call('HINCRBY', KEYS[11], 'quality_score', delta)
  local state_field = 'follow:user:' .. target_id
  local previous_state = redis.call('HGET', KEYS[16], state_field)
  local apply_state = true
  if previous_state then
    local decoded_state = cjson.decode(previous_state)
    local previous_time = tonumber(decoded_state.event_time) or 0
    local previous_client_event_id = tostring(decoded_state.client_event_id or '')
    if previous_time > event_time or
       (previous_time == event_time and previous_client_event_id >= client_event_id) then
      apply_state = false
    end
  end
  if apply_state then
    redis.call('HSET', KEYS[16], state_field, cjson.encode({
      event_time = event_time,
      client_event_id = client_event_id,
      action = action
    }))
    redis.call('EXPIRE', KEYS[16], ARGV[1])
    local author_posts = redis.call('ZREVRANGE', KEYS[13], 0, 99, 'WITHSCORES')
    if action == 'follow' then
      redis.call('SADD', KEYS[12], identity)
      for i = 1, #author_posts, 2 do
        redis.call('ZADD', KEYS[14], author_posts[i + 1], author_posts[i])
      end
    else
      redis.call('SREM', KEYS[12], identity)
      for i = 1, #author_posts, 2 do
        redis.call('ZREM', KEYS[14], author_posts[i])
      end
    end
  end
  redis.call('EXPIRE', KEYS[12], ARGV[1])
  redis.call('EXPIRE', KEYS[13], ARGV[1])
  redis.call('EXPIRE', KEYS[14], ARGV[1])
end
return 1
`

// RedisKeyLister 枚举 Redis 键（go-zero *redis.Redis 的 KeysCtx 满足该接口）。
type RedisKeyLister interface {
	KeysCtx(ctx context.Context, pattern string) ([]string, error)
}

// PurgeOptedOutFeatures 主动清理已关闭个性化用户的在线特征（REL-023）。
// 由 recommend-mq 定时任务周期调用：枚举 `personalization:optout:<userID>`
// 关闭标记并删除对应身份的全部特征键，确保关闭后 24 小时内删除在线特征，
// 不依赖用户后续是否再产生行为事件。
func (s *RedisBehaviorStore) PurgeOptedOutFeatures(ctx context.Context) (int, error) {
	if s == nil || s.redis == nil {
		return 0, nil
	}
	lister, ok := s.redis.(RedisKeyLister)
	if !ok {
		return 0, nil
	}
	keys, err := lister.KeysCtx(ctx, personalizationOptOutKeyPrefix+"*")
	if err != nil {
		return 0, fmt.Errorf("list personalization opt-out markers: %w", err)
	}
	purged := 0
	for _, key := range keys {
		userID, err := strconv.ParseInt(strings.TrimPrefix(key, personalizationOptOutKeyPrefix), 10, 64)
		if err != nil || userID <= 0 {
			// 非用户 ID 形式的标记不属于 REL-023 清理范围，跳过不视为失败。
			continue
		}
		if err := s.purgeIdentityFeatures(ctx, "u:"+strconv.FormatInt(userID, 10)); err != nil {
			return purged, fmt.Errorf("purge opted-out features for user %d: %w", userID, err)
		}
		purged++
	}
	return purged, nil
}
