package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var ErrConversationOwnedByAnother = errors.New("assistant conversation belongs to another user")

type Reference struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Title    string `json:"title,omitempty"`
	Snippet  string `json:"snippet,omitempty"`
	Revision int64  `json:"revision,omitempty"`
}

type Message struct {
	Role      string      `json:"role"`
	Content   string      `json:"content"`
	RequestID string      `json:"request_id"`
	Sources   []Reference `json:"sources,omitempty"`
	CreatedAt int64       `json:"created_at"`
}

type ConversationStore interface {
	Append(ctx context.Context, userID int64, conversationID string, message Message) error
}

type QuotaLimiter interface {
	Allow(ctx context.Context, userID int64) (bool, error)
}

type RedisEvaler interface {
	EvalCtx(ctx context.Context, script string, keys []string, args ...any) (any, error)
}

type RedisState struct {
	redis       RedisEvaler
	prefix      string
	ttlSeconds  int
	maxMessages int
	quotaWindow int
	quotaLimit  int
	now         func() time.Time
}

func NewRedisState(
	redis RedisEvaler,
	prefix string,
	ttlSeconds int,
	maxMessages int,
	quotaWindow int,
	quotaLimit int,
) (*RedisState, error) {
	if redis == nil {
		return nil, fmt.Errorf("assistant Redis client is required")
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || ttlSeconds <= 0 || maxMessages <= 0 || quotaWindow <= 0 || quotaLimit <= 0 {
		return nil, fmt.Errorf("assistant state configuration is invalid")
	}
	return &RedisState{
		redis: redis, prefix: prefix, ttlSeconds: ttlSeconds, maxMessages: maxMessages,
		quotaWindow: quotaWindow, quotaLimit: quotaLimit, now: time.Now,
	}, nil
}

func (s *RedisState) Append(ctx context.Context, userID int64, conversationID string, message Message) error {
	if userID <= 0 || strings.TrimSpace(conversationID) == "" {
		return fmt.Errorf("assistant conversation identity is invalid")
	}
	if message.Role != "user" && message.Role != "assistant" {
		return fmt.Errorf("assistant conversation role is invalid")
	}
	if strings.TrimSpace(message.Content) == "" || strings.TrimSpace(message.RequestID) == "" {
		return fmt.Errorf("assistant conversation message is incomplete")
	}
	if message.CreatedAt == 0 {
		message.CreatedAt = s.now().UnixMilli()
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal assistant conversation message: %w", err)
	}
	base := s.prefix + ":conversation:" + conversationID
	result, err := s.redis.EvalCtx(ctx, appendConversationScript,
		[]string{base + ":owner", base + ":messages"},
		userID, s.ttlSeconds, string(payload), s.maxMessages)
	if err != nil {
		return fmt.Errorf("append assistant conversation: %w", err)
	}
	value, err := integerResult(result)
	if err != nil {
		return fmt.Errorf("decode assistant conversation result: %w", err)
	}
	if value == -1 {
		return ErrConversationOwnedByAnother
	}
	if value != 1 {
		return fmt.Errorf("append assistant conversation returned %d", value)
	}
	return nil
}

func (s *RedisState) Allow(ctx context.Context, userID int64) (bool, error) {
	if userID <= 0 {
		return false, fmt.Errorf("assistant quota user is invalid")
	}
	key := fmt.Sprintf("%s:quota:%d", s.prefix, userID)
	result, err := s.redis.EvalCtx(ctx, quotaScript, []string{key}, s.quotaWindow)
	if err != nil {
		return false, fmt.Errorf("reserve assistant quota: %w", err)
	}
	count, err := integerResult(result)
	if err != nil {
		return false, fmt.Errorf("decode assistant quota result: %w", err)
	}
	return count <= int64(s.quotaLimit), nil
}

func integerResult(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected Redis result type %T", value)
	}
}

const appendConversationScript = `
local owner = redis.call('GET', KEYS[1])
if owner and owner ~= tostring(ARGV[1]) then
  return -1
end
if not owner then
  redis.call('SETEX', KEYS[1], ARGV[2], ARGV[1])
else
  redis.call('EXPIRE', KEYS[1], ARGV[2])
end
redis.call('RPUSH', KEYS[2], ARGV[3])
redis.call('LTRIM', KEYS[2], -tonumber(ARGV[4]), -1)
redis.call('EXPIRE', KEYS[2], ARGV[2])
return 1
`

const quotaScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return count
`
