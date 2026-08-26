package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// ConfirmBroker 实现 Agent 高危操作的逐次确认（AGNT-020~022）。
//
// 待确认项与裁决都以 Redis 键承载，跨实例可用：Runner 先 Open 再通过流发送
// CONFIRM_REQUIRED 事件，随后 Wait 轮询裁决键；ConfirmToolCall RPC 校验待确认
// 项归属后写入裁决键。凭据一次性：Wait 读到裁决后立即清理两个键，重放确认因
// pending 键缺失被拒绝。pending TTL 与确认等待上限一致，超时自然失效。
type ConfirmBroker interface {
	// Open 登记一条待确认操作；同一 callID 重复 Open 返回错误。
	Open(ctx context.Context, userID int64, requestID, callID, toolName, summary string, ttl time.Duration) error
	// Resolve 记录用户裁决（approved）；待确认项不存在或已过期返回 ErrConfirmExpired。
	Resolve(ctx context.Context, userID int64, requestID, callID string, approved bool) error
	// Wait 阻塞等待裁决；超时或 pending 失效返回 ErrConfirmExpired。
	Wait(ctx context.Context, userID int64, requestID, callID string, timeout time.Duration) (bool, error)
}

// ConfirmRedis 是 broker 依赖的 Redis 能力子集（go-zero redis.Redis 的子集），
// 便于测试注入。
type ConfirmRedis interface {
	GetCtx(ctx context.Context, key string) (string, error)
	SetexCtx(ctx context.Context, key, value string, seconds int) error
	DelCtx(ctx context.Context, keys ...string) (int, error)
	ExistsCtx(ctx context.Context, key string) (bool, error)
}

type redisConfirmBroker struct {
	redis     ConfirmRedis
	prefix    string
	pollEvery time.Duration
}

func NewRedisConfirmBroker(redis ConfirmRedis, stateKeyPrefix string) ConfirmBroker {
	prefix := stateKeyPrefix
	if prefix == "" {
		prefix = "assistant:v2"
	}
	return &redisConfirmBroker{redis: redis, prefix: prefix, pollEvery: 200 * time.Millisecond}
}

type pendingConfirmation struct {
	UserID      int64  `json:"user_id"`
	Tool        string `json:"tool"`
	Summary     string `json:"summary"`
	CreatedAtMs int64  `json:"created_at_ms"`
}

func (b *redisConfirmBroker) pendingKey(requestID, callID string) string {
	return fmt.Sprintf("%s:agent:pending:%s:%s", b.prefix, requestID, callID)
}

func (b *redisConfirmBroker) decisionKey(requestID, callID string) string {
	return fmt.Sprintf("%s:agent:decision:%s:%s", b.prefix, requestID, callID)
}

func (b *redisConfirmBroker) Open(ctx context.Context, userID int64, requestID, callID, toolName, summary string, ttl time.Duration) error {
	if userID <= 0 || requestID == "" || callID == "" || toolName == "" || ttl <= 0 {
		return fmt.Errorf("agent confirmation identity is invalid")
	}
	payload, err := json.Marshal(pendingConfirmation{
		UserID: userID, Tool: toolName, Summary: summary, CreatedAtMs: time.Now().UnixMilli(),
	})
	if err != nil {
		return err
	}
	return b.redis.SetexCtx(ctx, b.pendingKey(requestID, callID), string(payload), int(ttl/time.Second)+1)
}

func (b *redisConfirmBroker) Resolve(ctx context.Context, userID int64, requestID, callID string, approved bool) error {
	key := b.pendingKey(requestID, callID)
	raw, err := b.redis.GetCtx(ctx, key)
	if err != nil {
		return err
	}
	if raw == "" {
		return ErrConfirmExpired
	}
	var pending pendingConfirmation
	if json.Unmarshal([]byte(raw), &pending) != nil || pending.UserID != userID {
		return ErrConfirmExpired
	}
	value := "0"
	if approved {
		value = "1"
	}
	// 裁决键 TTL 与 pending 对齐；Wait 读取后主动清理。
	return b.redis.SetexCtx(ctx, b.decisionKey(requestID, callID), value, 300)
}

func (b *redisConfirmBroker) Wait(ctx context.Context, userID int64, requestID, callID string, timeout time.Duration) (bool, error) {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		exists, err := b.redis.ExistsCtx(ctx, b.pendingKey(requestID, callID))
		if err != nil {
			logx.WithContext(ctx).Errorw("agent confirm pending check failed", logx.Field("err", err.Error()))
		} else if !exists {
			b.cleanup(ctx, requestID, callID)
			return false, ErrConfirmExpired
		}
		decision, err := b.redis.GetCtx(ctx, b.decisionKey(requestID, callID))
		if err != nil {
			logx.WithContext(ctx).Errorw("agent confirm decision read failed", logx.Field("err", err.Error()))
		} else if decision != "" {
			b.cleanup(ctx, requestID, callID)
			return decision == "1", nil
		}
		if !time.Now().Before(deadline) {
			b.cleanup(ctx, requestID, callID)
			return false, ErrConfirmExpired
		}
		sleep := b.pollEvery
		if remaining := time.Until(deadline); remaining < sleep {
			sleep = remaining
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(sleep):
		}
	}
}

func (b *redisConfirmBroker) cleanup(ctx context.Context, requestID, callID string) {
	_, _ = b.redis.DelCtx(ctx, b.pendingKey(requestID, callID), b.decisionKey(requestID, callID))
}
