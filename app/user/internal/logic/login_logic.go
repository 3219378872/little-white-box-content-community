package logic

import (
	"context"
	"database/sql"
	"errors"
	"errx"
	"jwtx"
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
			return nil, err
		}
		if in.VerifyCode != verifyCode {
			return nil, errx.NewWithCode(errx.VerifyCodeError)
		}

		// 删除验证码
		_, err = l.svcCtx.RedisClient.DelCtx(l.ctx, in.Phone)
		if err != nil {
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
		if util.IsDefaultPassword(in.Password) {
			return nil, errx.New(errx.ParamError, "密码未设置，请使用手机登录并设置密码后登录")
		}
		// 校验信息
		if util.ComparePassword(user.Password, in.Password) != nil {
			return nil, errx.NewWithCode(errx.PasswordError)
		}
	}

	// 生成token
	token, err := jwtx.GenerateToken(user.Id, user.Username, l.svcCtx.Config.JwtConfig)
	if err != nil {
		l.Errorw("jwtx.GenerateToken failed",
			logx.Field("userId", user.Id),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	// 组装返回值
	return &pb.LoginResp{
		UserId: user.Id,
		Token:  token,
		User:   UserProfileToUserInfo(user),
	}, nil

}
