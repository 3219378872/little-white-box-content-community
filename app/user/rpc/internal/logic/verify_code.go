package logic

import (
	"context"

	"esx/app/user/rpc/internal/svc"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	verifyCodeMaxAttempts          = 5
	verifyCodeAttemptWindowSeconds = 600
)

// Validation, the failure limit and successful consumption share a Redis
// operation. No concurrent login or registration can reuse a successful code.
const consumeVerifyCodeScript = `
local current = redis.call('GET', KEYS[1])
if not current then return 0 end
if current ~= ARGV[1] then
  local attempts = redis.call('INCR', KEYS[2])
  if attempts == 1 then redis.call('EXPIRE', KEYS[2], ARGV[3]) end
  if attempts >= tonumber(ARGV[2]) then redis.call('DEL', KEYS[1]) end
  return -1
end
redis.call('DEL', KEYS[1], KEYS[2])
return 1
`

func consumeVerifyCode(ctx context.Context, rds svc.RedisStore, phone, code string) error {
	if rds == nil {
		return errx.NewWithCode(errx.SystemError)
	}
	result, err := rds.EvalCtx(ctx, consumeVerifyCodeScript,
		[]string{verifyCodeRedisKey(phone), "verify:attempts:" + phone},
		code, verifyCodeMaxAttempts, verifyCodeAttemptWindowSeconds)
	if err != nil {
		logx.WithContext(ctx).Errorw("consume verification code failed", logx.Field("err", err.Error()))
		return errx.NewWithCode(errx.SystemError)
	}
	consumed, err := redisInteger(result)
	if err != nil {
		return errx.NewWithCode(errx.SystemError)
	}
	switch consumed {
	case 1:
		return nil
	case 0:
		return errx.NewWithCode(errx.VerifyCodeExpired)
	case -1:
		return errx.NewWithCode(errx.VerifyCodeError)
	default:
		return errx.NewWithCode(errx.SystemError)
	}
}

func verifyCodeRedisKey(phone string) string { return "verify:code:" + phone }
