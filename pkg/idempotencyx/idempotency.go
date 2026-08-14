package idempotencyx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// maxIdempotencyKeySize 与 CORE-042/050 一致：客户端幂等键最长 128 字符。
const maxIdempotencyKeySize = 128

// ErrIdempotencyConflict 表示同一幂等键已被绑定到不同命令。
var ErrIdempotencyConflict = errors.New("idempotency key is already bound to another command")

// IdempotencyRecord 描述一次命令幂等绑定。
type IdempotencyRecord struct {
	Scope       string
	UserID      int64
	Key         string
	CommandHash string
}

// CommandHash 对命令参数生成 sha256 指纹，用于区分同键异命令。
func CommandHash(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Valid 校验幂等记录；空 Key 表示调用方未提供幂等键（不做去重）。
func (r IdempotencyRecord) Valid() bool {
	if r.Key == "" {
		return true
	}
	return r.Scope != "" && r.UserID > 0 && len(r.Key) <= maxIdempotencyKeySize && r.CommandHash != ""
}

// resolveIdempotencySession 在既有事务会话内解析幂等键：
//   - Key 为空：返回 created=true 与 newResourceID，调用方继续创建。
//   - 已存在且命令一致：返回既有 resourceID 与 created=false，调用方跳过创建。
//   - 已存在但命令不一致：返回 ErrIdempotencyConflict。
//
// 并发同键竞争由唯一键约束处理：重复键错误后回查胜出事务的绑定。
func ResolveIdempotencySession(
	ctx context.Context,
	session sqlx.Session,
	rec IdempotencyRecord,
	recordID int64,
	newResourceID int64,
) (resourceID int64, created bool, err error) {
	if rec.Key == "" {
		return newResourceID, true, nil
	}
	if !rec.Valid() {
		return 0, false, fmt.Errorf("idempotencyx: invalid idempotency record")
	}
	existing, found, err := findIdempotencySession(ctx, session, rec.Scope, rec.UserID, rec.Key)
	if err != nil {
		return 0, false, err
	}
	if found {
		if existing.CommandHash != rec.CommandHash {
			return 0, false, ErrIdempotencyConflict
		}
		return existing.ResourceID, false, nil
	}
	_, err = session.ExecCtx(ctx,
		"INSERT INTO `idempotency` (`id`, `scope`, `user_id`, `key`, `command_hash`, `resource_id`, `created_at`) VALUES (?, ?, ?, ?, ?, ?, ?)",
		recordID, rec.Scope, rec.UserID, rec.Key, rec.CommandHash, newResourceID, time.Now().UnixMilli())
	if err == nil {
		return newResourceID, true, nil
	}
	if !isDuplicateKeyError(err) {
		return 0, false, err
	}
	existing, found, lookupErr := findIdempotencySession(ctx, session, rec.Scope, rec.UserID, rec.Key)
	if lookupErr != nil {
		return 0, false, lookupErr
	}
	if !found {
		return 0, false, err
	}
	if existing.CommandHash != rec.CommandHash {
		return 0, false, ErrIdempotencyConflict
	}
	return existing.ResourceID, false, nil
}

type storedIdempotency struct {
	ResourceID  int64  `db:"resource_id"`
	CommandHash string `db:"command_hash"`
}

func findIdempotencySession(ctx context.Context, session sqlx.Session, scope string, userID int64, key string) (storedIdempotency, bool, error) {
	var stored storedIdempotency
	err := session.QueryRowCtx(ctx, &stored,
		"SELECT `resource_id`, `command_hash` FROM `idempotency` WHERE `scope` = ? AND `user_id` = ? AND `key` = ? LIMIT 1",
		scope, userID, key)
	if errors.Is(err, sqlx.ErrNotFound) {
		return storedIdempotency{}, false, nil
	}
	if err != nil {
		return storedIdempotency{}, false, err
	}
	return stored, true, nil
}

func isDuplicateKeyError(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
