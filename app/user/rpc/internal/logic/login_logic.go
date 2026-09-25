package logic

import (
	"context"
	"database/sql"
	"errors"
	"esx/app/user/rpc/internal/model"
	"esx/app/user/rpc/internal/password"
	"esx/app/user/rpc/internal/svc"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/errx"
	"fmt"

	"github.com/zeromicro/go-zero/core/logx"
)

type LoginLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 登录
func (l *LoginLogic) Login(in *pb.LoginReq) (*pb.LoginResp, error) {
	var user *model.UserProfile
	var err error
	// 1.密码登录 2.验证码登录
	if in.LoginType == 2 {
		user, err = l.svcCtx.UserProfileModel.FindOneByPhone(l.ctx, sql.NullString{
			String: in.Phone,
			Valid:  true,
		})
		if err != nil {
			if errors.Is(err, model.ErrNotFound) {
				return nil, errx.NewWithCode(errx.UserNotFound)
			}
			return nil, errx.NewWithCode(errx.SystemError)
		}

		if err := consumeVerifyCode(l.ctx, l.svcCtx.RedisClient, in.Phone, in.VerifyCode); err != nil {
			return nil, err
		}
	} else {
		user, err = l.svcCtx.UserProfileModel.FindOneByUsername(l.ctx, in.Username)
		if err != nil {
			if errors.Is(err, model.ErrNotFound) {
				return nil, errx.NewWithCode(errx.UserNotFound)
			}
			return nil, errx.New(errx.SystemError, "系统错误，请稍后再试")
		}
		// 密码登录时，检查是否为默认密码，若是则拒绝
		if password.IsDefault(in.Password) {
			return nil, errx.New(errx.ParamError, "密码未设置，请使用手机登录并设置密码后登录")
		}
		// 先看锁定：窗口内错误次数已达上限则拒绝，正确密码也不能登录。
		if locked, lockErr := l.loginLocked(user.Id); lockErr != nil {
			l.Errorw("login lock check failed",
				logx.Field("username", in.Username), logx.Field("err", lockErr.Error()))
		} else if locked {
			return nil, errx.NewWithCode(errx.TooManyReq)
		}
		if password.Compare(user.Password, in.Password) != nil {
			if recErr := l.recordLoginFailure(user.Id); recErr != nil {
				l.Errorw("login failure record failed",
					logx.Field("username", in.Username), logx.Field("err", recErr.Error()))
			}
			return nil, errx.NewWithCode(errx.PasswordError)
		}
		if l.svcCtx.RedisClient != nil {
			_, _ = l.svcCtx.RedisClient.DelCtx(l.ctx, loginLockKey(user.Id))
		}
	}

	// 签发访问/刷新令牌对
	token, refreshToken, err := issueTokenPair(l.ctx, l.svcCtx, user.Id, user.Username)
	if err != nil {
		return nil, err
	}

	// 组装返回值
	return &pb.LoginResp{
		UserId:       user.Id,
		Token:        token,
		User:         UserProfileToUserInfo(user),
		RefreshToken: refreshToken,
	}, nil

}

// The database username comparison is case/accent insensitive. Use the resolved
// account ID so every spelling of one account shares the same failure window.
func loginLockKey(userID int64) string {
	return fmt.Sprintf("login:lock:user:%d", userID)
}

// Both reads and increments repair counters without TTL. The check must repair
// an already locked counter too, since such a request never reaches INCR.
const loginLockCheckScript = `
local attempts = redis.call('GET', KEYS[1])
if not attempts then return 0 end
if redis.call('TTL', KEYS[1]) < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return tonumber(attempts)
`

const loginLockFailureScript = `
local attempts = redis.call('INCR', KEYS[1])
if redis.call('TTL', KEYS[1]) < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return attempts
`

// loginLocked reads and repairs the failure window atomically. The caller keeps
// the existing fail-open policy for Redis outages.
func (l *LoginLogic) loginLocked(userID int64) (bool, error) {
	if l.svcCtx == nil || l.svcCtx.RedisClient == nil || userID <= 0 {
		return false, nil
	}
	result, err := l.svcCtx.RedisClient.EvalCtx(l.ctx, loginLockCheckScript,
		[]string{loginLockKey(userID)}, loginLockWindowSeconds)
	if err != nil {
		return false, err
	}
	attempts, err := redisInteger(result)
	return attempts >= loginLockMaxAttempts, err
}

func (l *LoginLogic) recordLoginFailure(userID int64) error {
	if l.svcCtx == nil || l.svcCtx.RedisClient == nil || userID <= 0 {
		return nil
	}
	_, err := l.svcCtx.RedisClient.EvalCtx(l.ctx, loginLockFailureScript,
		[]string{loginLockKey(userID)}, loginLockWindowSeconds)
	return err
}

// 密码登录失败锁定：窗口内允许的错误次数。
const (
	loginLockMaxAttempts   = 5
	loginLockWindowSeconds = 600
)
