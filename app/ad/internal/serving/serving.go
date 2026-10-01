// Package serving 维护投放索引、频控、隐藏与请求结果缓存（DES-sponsored-ads「投放」）。
//
// Redis 键均带 ads:v1: 前缀，只服务频控与用户屏蔽，不进入个性化特征。身份为 u:<userId>，
// 匿名为 s:<sessionId 哈希>，只按会话计数，不使用 anonymousId（ADS-024、ADS-026、DISC-031）。
package serving

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

const (
	prefix = "ads:v1:"
	// DailyCap 是同一身份对同一广告每个 UTC 自然日的最大下发次数（ADS-024）。
	DailyCap = 3
	// HideTTL 是隐藏后不再投放的期限（ADS-026）。
	HideTTL = 30 * 24 * time.Hour
	// SessionHideTTL 是匿名会话隐藏键的保留期；会话结束后自然失效。
	SessionHideTTL = 24 * time.Hour
	// RequestTTL 保证客户端重试得到相同广告且不重复计频。
	RequestTTL = 10 * time.Minute
	freqTTL    = 48 * time.Hour
)

// ErrNoIdentity 表示请求既无用户也无会话，不能计频，因此不返回广告。
var ErrNoIdentity = errors.New("serving: no identity")

// KV 是 Redis 的最小子集。
type KV interface {
	EvalCtx(ctx context.Context, script string, keys []string, args ...any) (any, error)
	GetCtx(ctx context.Context, k string) (string, error)
	SetexCtx(ctx context.Context, k, v string, seconds int) error
	HgetallCtx(ctx context.Context, k string) (map[string]string, error)
}

// Entry 是投放索引中的一条可投广告。
type Entry struct {
	AdID         int64  `json:"adId"`
	Revision     int64  `json:"revision"`
	AdvertiserID int64  `json:"advertiserId"`
	Industry     string `json:"industry"`
	StartMs      int64  `json:"startMs"`
	EndMs        int64  `json:"endMs"`
}

// Identity 返回频控与隐藏使用的身份键。
func Identity(userID int64, sessionID string) (string, error) {
	if userID > 0 {
		return "u:" + strconv.FormatInt(userID, 10), nil
	}
	if sessionID == "" {
		return "", ErrNoIdentity
	}
	sum := sha256.Sum256([]byte(sessionID))
	return "s:" + hex.EncodeToString(sum[:12]), nil
}

func IndexKey(market string) string { return prefix + "serving:" + market }

func FreqKey(identity string, now time.Time) string {
	return prefix + "freq:" + identity + ":" + now.UTC().Format("20060102")
}

func HiddenKey(identity string) string { return prefix + "hidden:" + identity }

func RequestKey(requestID, cursor string) string {
	sum := sha256.Sum256([]byte(cursor))
	return prefix + "req:" + requestID + ":" + hex.EncodeToString(sum[:8])
}

// Index 读写投放索引。
type Index struct{ KV KV }

const upsertScript = `redis.call('HSET', KEYS[1], ARGV[1], ARGV[2]) return 1`
const removeScript = `return redis.call('HDEL', KEYS[1], ARGV[1])`

// Upsert 把过审且已发布素材的广告放入市场索引。
func (i Index) Upsert(ctx context.Context, market string, entry Entry) error {
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = i.KV.EvalCtx(ctx, upsertScript, []string{IndexKey(market)}, strconv.FormatInt(entry.AdID, 10), string(raw))
	return err
}

// Remove 把广告移出市场索引（暂停、下线、资质失效，ADS-032）。
func (i Index) Remove(ctx context.Context, market string, adID int64) error {
	_, err := i.KV.EvalCtx(ctx, removeScript, []string{IndexKey(market)}, strconv.FormatInt(adID, 10))
	return err
}

// Load 读取市场索引。
func (i Index) Load(ctx context.Context, market string) ([]Entry, error) {
	raw, err := i.KV.HgetallCtx(ctx, IndexKey(market))
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(raw))
	for _, value := range raw {
		var entry Entry
		if err := json.Unmarshal([]byte(value), &entry); err != nil || entry.AdID <= 0 {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// counters 返回 field → 整数值。
func counters(ctx context.Context, kv KV, key string) (map[int64]int64, error) {
	raw, err := kv.HgetallCtx(ctx, key)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]int64, len(raw))
	for field, value := range raw {
		id, err1 := strconv.ParseInt(field, 10, 64)
		n, err2 := strconv.ParseInt(value, 10, 64)
		if err1 == nil && err2 == nil {
			out[id] = n
		}
	}
	return out, nil
}

// reserveScript 原子地为一组广告计频：任一广告已达上限则跳过该广告，返回实际计入的广告 ID。
// 计数与过期在同一脚本内完成（DES：现有验证码计数 INCR 与 EXPIRE 分两步，这里不沿用）。
const reserveScript = `
local granted = {}
for i = 1, #ARGV - 2 do
  local count = tonumber(redis.call('HGET', KEYS[1], ARGV[i]) or '0')
  if count < tonumber(ARGV[#ARGV - 1]) then
    redis.call('HINCRBY', KEYS[1], ARGV[i], 1)
    table.insert(granted, ARGV[i])
  end
end
redis.call('EXPIRE', KEYS[1], tonumber(ARGV[#ARGV]))
return granted`

// Reserve 为选中的广告计频，返回仍在上限内的广告。
func Reserve(ctx context.Context, kv KV, identity string, adIDs []int64, now time.Time) ([]int64, error) {
	if len(adIDs) == 0 {
		return nil, nil
	}
	args := make([]any, 0, len(adIDs)+2)
	for _, id := range adIDs {
		args = append(args, strconv.FormatInt(id, 10))
	}
	args = append(args, DailyCap, int(freqTTL.Seconds()))
	result, err := kv.EvalCtx(ctx, reserveScript, []string{FreqKey(identity, now)}, args...)
	if err != nil {
		return nil, err
	}
	values, ok := result.([]any)
	if !ok {
		return nil, fmt.Errorf("serving: unexpected reserve result %T", result)
	}
	granted := make([]int64, 0, len(values))
	for _, v := range values {
		s, _ := v.(string)
		if id, err := strconv.ParseInt(s, 10, 64); err == nil {
			granted = append(granted, id)
		}
	}
	return granted, nil
}

const hideScript = `redis.call('HSET', KEYS[1], ARGV[1], ARGV[2]) redis.call('EXPIRE', KEYS[1], tonumber(ARGV[3])) return 1`

// Hide 记录用户隐藏（ADS-026）：已认证用户 30 天；匿名用户只在当前会话内生效。
func Hide(ctx context.Context, kv KV, identity string, adID int64, now time.Time) error {
	ttl := HideTTL
	if identity[0] == 's' {
		ttl = SessionHideTTL
	}
	until := now.Add(ttl).UnixMilli()
	_, err := kv.EvalCtx(ctx, hideScript, []string{HiddenKey(identity)}, strconv.FormatInt(adID, 10),
		strconv.FormatInt(until, 10), int(ttl.Seconds()))
	return err
}

// Hidden 返回仍在隐藏期内的广告。
func Hidden(ctx context.Context, kv KV, identity string, now time.Time) (map[int64]bool, error) {
	raw, err := counters(ctx, kv, HiddenKey(identity))
	if err != nil {
		return nil, err
	}
	out := make(map[int64]bool, len(raw))
	for id, until := range raw {
		if until > now.UnixMilli() {
			out[id] = true
		}
	}
	return out, nil
}

// Counts 返回当日各广告的下发次数。
func Counts(ctx context.Context, kv KV, identity string, now time.Time) (map[int64]int64, error) {
	return counters(ctx, kv, FreqKey(identity, now))
}
