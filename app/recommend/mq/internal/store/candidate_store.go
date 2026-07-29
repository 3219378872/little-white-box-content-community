package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"esx/pkg/event"
)

type CandidateStore interface {
	RecordPost(ctx context.Context, post event.PostEvent) error
}

type RedisCandidateStore struct {
	redis          RedisEvaler
	featureVersion string
	recallPrefix   string
	ttlSeconds     int
}

func NewRedisCandidateStore(
	redis RedisEvaler,
	featureVersion string,
	recallKeyPrefix string,
	ttlSeconds int,
) *RedisCandidateStore {
	return &RedisCandidateStore{
		redis: redis, featureVersion: featureVersion,
		recallPrefix: recallKeyPrefix + ":" + featureVersion, ttlSeconds: ttlSeconds,
	}
}

func (s *RedisCandidateStore) RecordPost(ctx context.Context, post event.PostEvent) error {
	if err := post.Validate(); err != nil {
		return fmt.Errorf("validate recommendation post event: %w", err)
	}
	category := ""
	if len(post.Tags) > 0 {
		category = post.Tags[0]
	}
	postID := strconv.FormatInt(post.PostID, 10)
	authorID := strconv.FormatInt(post.AuthorID, 10)
	_, err := s.redis.EvalCtx(ctx, recordPostCandidateScript, []string{
		"feature:" + s.featureVersion + ":post:" + postID,
		s.recallPrefix + ":recall:post:hot:home",
		s.recallPrefix + ":recall:post:explore:home",
		s.recallPrefix + ":author:" + authorID + ":posts",
		s.recallPrefix + ":follow:author:" + authorID + ":followers",
	}, string(post.Type), postID, authorID, category, post.EventTime,
		s.ttlSeconds, s.recallPrefix)
	if err != nil {
		return fmt.Errorf("record recommendation post candidates: %w", err)
	}
	return nil
}

const recordPostCandidateScript = `
local event_type = ARGV[1]
local post_id = ARGV[2]
local author_id = ARGV[3]
local category = ARGV[4]
local event_time = ARGV[5]
local ttl = ARGV[6]
local recall_prefix = ARGV[7]

if event_type == 'post.deleted' then
  redis.call('HSET', KEYS[1], 'status', 'deleted')
  redis.call('ZREM', KEYS[2], post_id)
  redis.call('ZREM', KEYS[3], post_id)
  redis.call('ZREM', KEYS[4], post_id)
else
  redis.call('HSET', KEYS[1],
    'status', 'active', 'visibility', 'public', 'author_id', author_id,
    'category', category, 'quality_score', '0.5', 'freshness', '1')
  redis.call('HSETNX', KEYS[1], 'popularity', '0')
  redis.call('HSETNX', KEYS[1], 'ctr', '0')
  redis.call('ZADD', KEYS[2], event_time, post_id)
  redis.call('ZADD', KEYS[3], event_time, post_id)
  redis.call('ZADD', KEYS[4], event_time, post_id)
end

local followers = redis.call('SMEMBERS', KEYS[5])
for _, identity in ipairs(followers) do
  local follow_key = recall_prefix .. ':recall:post:follow:' .. identity .. ':home'
  if event_type == 'post.deleted' then
    redis.call('ZREM', follow_key, post_id)
  else
    redis.call('ZADD', follow_key, event_time, post_id)
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
