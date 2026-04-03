package logic

import (
	"context"
	"database/sql"
	"jwtx"
	"user/internal/model"
	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type LoginLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

const PasswordType = 1
const PhoneType = 2

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
	if in.LoginType == PhoneType {
		user, err = l.svcCtx.UserProfileModel.FindOneByPhone(l.ctx, sql.NullString{
			String: in.Phone,
			Valid:  true,
		})
		if err != nil {
			return nil, err
		}
	} else {
		user, err = l.svcCtx.UserProfileModel.FindOneByUsername(l.ctx, in.Username)
		if err != nil {
			return nil, err
		}
	}

	// 生成token
	token, err := jwtx.GenerateToken(user.Id, user.Username, l.svcCtx.Config.JwtConfig)
	if err != nil {
		logx.Errorf("token生成失败")
	}

	// 组装返回值
	return &pb.LoginResp{
		UserId: user.Id,
		Token:  token,
		User:   UserProfileToUserInfo(user),
	}, nil

}
