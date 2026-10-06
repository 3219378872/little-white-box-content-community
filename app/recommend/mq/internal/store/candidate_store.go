package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"esx/app/recommend/featurekey"
	"esx/pkg/event"
	"esx/pkg/visibilityx"
)

type CandidateStore interface {
	RecordPost(ctx context.Context, post event.PostEvent) error
}

type RedisCandidateStore struct {
	privacy      *RedisBehaviorStore
	redis        RedisEvaler
	features     featurekey.Space
	recallPrefix string
	ttlSeconds   int
}

func NewRedisCandidateStore(
	redis RedisEvaler,
	featureVersion string,
	recallKeyPrefix string,
	ttlSeconds int,
	readers ...PersonalizationPreferenceReader,
) *RedisCandidateStore {
	return &RedisCandidateStore{
		privacy: NewRedisBehaviorStore(redis, featureVersion, recallKeyPrefix, ttlSeconds, readers...),
		redis:   redis, features: featurekey.New(featureVersion),
		recallPrefix: recallKeyPrefix + ":" + featureVersion, ttlSeconds: ttlSeconds,
	}
}

func (s *RedisCandidateStore) RecordPost(ctx context.Context, post event.PostEvent) error {
	if err := post.Validate(); err != nil {
		return fmt.Errorf("validate recommendation post event: %w", err)
	}
	// Count patches carry no author, tags or revision; treating them as a
	// snapshot would erase the author, reset the revision guard and revive
	// deleted candidates. Popularity comes from behavior events instead.
	if post.Type == event.PostEventCounted {
		return nil
	}
	status := candidateStatus(post)
	category := ""
	if len(post.Tags) > 0 {
		category = post.Tags[0]
	}
	postID := strconv.FormatInt(post.PostID, 10)
	authorID := strconv.FormatInt(post.AuthorID, 10)
	// Post fan-out also derives per-user recommendations. Re-check durable
	// consent instead of trusting follower membership left by an older event.
	allowed := map[string]bool{}
	if members, ok := s.redis.(interface {
		SmembersCtx(context.Context, string) ([]string, error)
	}); ok && status == candidateActive {
		followers, err := members.SmembersCtx(ctx, s.recallPrefix+":follow:author:"+authorID+":followers")
		if err != nil {
			return fmt.Errorf("load candidate followers: %w", err)
		}
		for _, identity := range followers {
			// 只有登录用户有可复核的个性化偏好；匿名身份不参与个性化扇出。
			if !strings.HasPrefix(identity, featurekey.UserIdentityPrefix) {
				continue
			}
			userID, err := strconv.ParseInt(strings.TrimPrefix(identity, featurekey.UserIdentityPrefix), 10, 64)
			if err != nil || userID <= 0 {
				continue
			}
			optedOut, err := s.privacy.personalizationOptedOut(ctx, userID)
			if err != nil {
				return fmt.Errorf("check candidate follower preference: %w", err)
			}
			if !optedOut {
				allowed[identity] = true
			}
		}
	}
	allowedJSON, err := json.Marshal(allowed)
	if err != nil {
		return fmt.Errorf("marshal candidate preferences: %w", err)
	}
	_, err = s.redis.EvalCtx(ctx, recordPostCandidateScript, []string{
		s.features.Post(post.PostID),
		s.recallPrefix + ":recall:post:hot:home",
		s.recallPrefix + ":recall:post:explore:home",
		s.recallPrefix + ":author:" + authorID + ":posts",
		s.recallPrefix + ":follow:author:" + authorID + ":followers",
	}, status, postID, authorID, category, post.EventTime,
		s.ttlSeconds, s.recallPrefix, post.Revision, string(allowedJSON))
	if err != nil {
		return fmt.Errorf("record recommendation post candidates: %w", err)
	}
	return nil
}

const (
	candidateActive      = "active"
	candidateDeleted     = "deleted"
	candidateUnpublished = "unpublished"
)

// candidateStatus maps a post snapshot to its candidate lifecycle. Drafts and
// unpublished posts leave recall like deletions (CORE-015); a later publish
// carries a newer revision and restores them.
func candidateStatus(post event.PostEvent) string {
	if post.Type == event.PostEventDeleted {
		return candidateDeleted
	}
	if !visibilityx.IsPublished(post.Status) {
		return candidateUnpublished
	}
	return candidateActive
}

const recordPostCandidateScript = `
local status = ARGV[1]
local post_id = ARGV[2]
local author_id = ARGV[3]
local category = ARGV[4]
local event_time = ARGV[5]
local ttl = ARGV[6]
local recall_prefix = ARGV[7]
local incoming_rev = tonumber(ARGV[8]) or 0
local stored_rev = tonumber(redis.call('HGET', KEYS[1], 'revision') or '0')
if incoming_rev > 0 and incoming_rev <= stored_rev then
  return 0
end

if status ~= 'active' then
  redis.call('HSET', KEYS[1], 'status', status, 'revision', incoming_rev)
  redis.call('ZREM', KEYS[2], post_id)
  redis.call('ZREM', KEYS[3], post_id)
  redis.call('ZREM', KEYS[4], post_id)
else
  redis.call('HSET', KEYS[1],
    'status', 'active', 'visibility', 'public', 'author_id', author_id,
    'category', category, 'quality_score', '0.5', 'freshness', '1',
    'revision', incoming_rev)
  redis.call('HSETNX', KEYS[1], 'popularity', '0')
  redis.call('HSETNX', KEYS[1], 'ctr', '0')
  redis.call('ZADD', KEYS[2], event_time, post_id)
  redis.call('ZADD', KEYS[3], event_time, post_id)
  redis.call('ZADD', KEYS[4], event_time, post_id)
end

local allowed_followers = cjson.decode(ARGV[9])
local followers = redis.call('SMEMBERS', KEYS[5])
for _, identity in ipairs(followers) do
  local follow_key = recall_prefix .. ':recall:post:follow:' .. identity .. ':home'
  if status ~= 'active' then
    redis.call('ZREM', follow_key, post_id)
  elseif allowed_followers[identity] then
    redis.call('ZADD', follow_key, event_time, post_id)
  else
    redis.call('DEL', follow_key)
    redis.call('SREM', KEYS[5], identity)
  end
  redis.call('EXPIRE', follow_key, ttl)
end
redis.call('EXPIRE', KEYS[2], ttl)
redis.call('EXPIRE', KEYS[3], ttl)
redis.call('EXPIRE', KEYS[4], ttl)
redis.call('EXPIRE', KEYS[5], ttl)
return 1
`

type DeadLetter struct {
	MessageID  string `json:"message_id"`
	Payload    []byte `json:"payload"`
	Error      string `json:"error"`
	RecordedAt int64  `json:"recorded_at"`
}

type DeadLetterRecorder interface {
	RecordDeadLetter(ctx context.Context, messageID string, payload []byte, cause error) error
}

type RedisDeadLetterRecorder struct {
	redis      RedisEvaler
	key        string
	ttlSeconds int
	maxLength  int
	now        func() time.Time
}

func NewRedisDeadLetterRecorder(
	redis RedisEvaler,
	recallKeyPrefix string,
	featureVersion string,
	ttlSeconds int,
	maxLength int,
) *RedisDeadLetterRecorder {
	return &RedisDeadLetterRecorder{
		redis: redis, key: recallKeyPrefix + ":" + featureVersion + ":dead-letters",
		ttlSeconds: ttlSeconds, maxLength: maxLength, now: time.Now,
	}
}

func (r *RedisDeadLetterRecorder) RecordDeadLetter(
	ctx context.Context,
	messageID string,
	payload []byte,
	cause error,
) error {
	if messageID == "" {
		messageID = "unknown"
	}
	message := "invalid event"
	if cause != nil {
		message = cause.Error()
	}
	letter, err := json.Marshal(DeadLetter{
		MessageID: messageID, Payload: payload, Error: message, RecordedAt: r.now().UnixMilli(),
	})
	if err != nil {
		return fmt.Errorf("marshal recommendation dead letter: %w", err)
	}
	_, err = r.redis.EvalCtx(ctx, recordDeadLetterScript, []string{r.key},
		string(letter), r.maxLength, r.ttlSeconds)
	if err != nil {
		return fmt.Errorf("record recommendation dead letter: %w", err)
	}
	return nil
}

const recordDeadLetterScript = `
redis.call('LPUSH', KEYS[1], ARGV[1])
redis.call('LTRIM', KEYS[1], 0, tonumber(ARGV[2]) - 1)
redis.call('EXPIRE', KEYS[1], ARGV[3])
return 1
`
