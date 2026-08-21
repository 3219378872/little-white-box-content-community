package logic

import (
	"context"
	"database/sql"
	"errors"
	"errx"
	"fmt"
	"strings"
	"user/internal/model"
	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"
	"util"

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
		verifyCode, err := l.svcCtx.RedisClient.GetCtx(l.ctx, in.Phone)
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
		_, err = l.svcCtx.RedisClient.DelCtx(l.ctx, in.Phone)
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
		if util.IsDefaultPassword(in.Password) {
			return nil, errx.New(errx.ParamError, "密码未设置，请使用手机登录并设置密码后登录")
		}
		// 校验信息
		if util.ComparePassword(user.Password, in.Password) != nil {
			// 密码失败锁定：窗口内错误次数达到上限后拒绝，防暴力破解。
			if lockOut, lockErr := l.loginFailureLocked(in.Username); lockErr != nil {
				l.Errorw("login failure lock check failed",
					logx.Field("username", in.Username), logx.Field("err", lockErr.Error()))
			} else if lockOut {
				return nil, errx.NewWithCode(errx.TooManyReq)
			}
			return nil, errx.NewWithCode(errx.PasswordError)
		}
		if l.svcCtx.RedisClient != nil {
			_, _ = l.svcCtx.RedisClient.DelCtx(l.ctx, fmt.Sprintf("login:lock:%s", in.Username))
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

// loginFailureLocked 记录一次密码登录失败；窗口内达到上限返回 true（锁定）。
// 成功后由调用方清理计数。
func (l *LoginLogic) loginFailureLocked(username string) (bool, error) {
	if l.svcCtx == nil || l.svcCtx.RedisClient == nil || strings.TrimSpace(username) == "" {
		return false, nil
	}
	lockKey := fmt.Sprintf("login:lock:%s", username)
	attempts, err := l.svcCtx.RedisClient.IncrCtx(l.ctx, lockKey)
	if err != nil {
		return false, err
	}
	if attempts == 1 {
		_ = l.svcCtx.RedisClient.ExpireCtx(l.ctx, lockKey, loginLockWindowSeconds)
	}
	return attempts >= loginLockMaxAttempts, nil
}

// 密码登录失败锁定：窗口内允许的错误次数。
const (
	loginLockMaxAttempts   = 5
	loginLockWindowSeconds = 600
)
