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
	"strconv"
	"strings"

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

		// 校验信息
		verifyCode, err := l.svcCtx.RedisClient.GetCtx(l.ctx, verifyCodeRedisKey(in.Phone))
		if err != nil {
			l.Errorw("Redis.GetCtx failed", logx.Field("err", err.Error()))
			return nil, errx.Wrap(err, errx.SystemError)
		}
		if verifyCode == "" {
			// 与注册语义一致：验证码不存在/已过期与"错误"区分开。
			return nil, errx.NewWithCode(errx.VerifyCodeExpired)
		}
		if in.VerifyCode != verifyCode {
			// 与注册共享验证码尝试计数：总错误次数受限，防暴力破解登录。
			recordVerifyCodeFailure(l.ctx, l.svcCtx.RedisClient, in.Phone)
			return nil, errx.NewWithCode(errx.VerifyCodeError)
		}
		clearVerifyCodeFailures(l.ctx, l.svcCtx.RedisClient, in.Phone)

		// 删除验证码
		_, err = l.svcCtx.RedisClient.DelCtx(l.ctx, verifyCodeRedisKey(in.Phone))
		if err != nil {
			l.Errorw("Redis.DelCtx failed", logx.Field("err", err.Error()))
			return nil, errx.Wrap(err, errx.SystemError)
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
		if locked, lockErr := l.loginLocked(in.Username); lockErr != nil {
			l.Errorw("login lock check failed",
				logx.Field("username", in.Username), logx.Field("err", lockErr.Error()))
		} else if locked {
			return nil, errx.NewWithCode(errx.TooManyReq)
		}
		if password.Compare(user.Password, in.Password) != nil {
			if recErr := l.recordLoginFailure(in.Username); recErr != nil {
				l.Errorw("login failure record failed",
					logx.Field("username", in.Username), logx.Field("err", recErr.Error()))
			}
			return nil, errx.NewWithCode(errx.PasswordError)
		}
		if l.svcCtx.RedisClient != nil {
			_, _ = l.svcCtx.RedisClient.DelCtx(l.ctx, loginLockKey(in.Username))
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

func loginLockKey(username string) string {
	return fmt.Sprintf("login:lock:%s", username)
}

// loginLocked 读取当前失败计数；窗口内达到上限返回 true。Redis 故障由调用方 fail-open。
func (l *LoginLogic) loginLocked(username string) (bool, error) {
	if l.svcCtx == nil || l.svcCtx.RedisClient == nil || strings.TrimSpace(username) == "" {
		return false, nil
	}
	raw, err := l.svcCtx.RedisClient.GetCtx(l.ctx, loginLockKey(username))
	if err != nil {
		return false, err
	}
	if raw == "" {
		return false, nil
	}
	attempts, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return false, nil
	}
	return attempts >= loginLockMaxAttempts, nil
}

// recordLoginFailure 仅在密码比对失败后递增计数；首次写入时设置窗口 TTL。
func (l *LoginLogic) recordLoginFailure(username string) error {
	if l.svcCtx == nil || l.svcCtx.RedisClient == nil || strings.TrimSpace(username) == "" {
		return nil
	}
	lockKey := loginLockKey(username)
	attempts, err := l.svcCtx.RedisClient.IncrCtx(l.ctx, lockKey)
	if err != nil {
		return err
	}
	if attempts == 1 {
		_ = l.svcCtx.RedisClient.ExpireCtx(l.ctx, lockKey, loginLockWindowSeconds)
	}
	return nil
}

// 密码登录失败锁定：窗口内允许的错误次数。
const (
	loginLockMaxAttempts   = 5
	loginLockWindowSeconds = 600
)
