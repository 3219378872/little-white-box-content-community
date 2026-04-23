// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package login

import (
	"context"
	"errx"
	"esx/pkg/validator"
	"gateway/internal/svc"
	"gateway/internal/types"
	"user/userservice"

	"github.com/zeromicro/go-zero/core/logx"
)

type LoginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 用户登录
func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LoginLogic) Login(req *types.LoginReq) (resp *types.LoginResp, err error) {
	if req.LoginType == 2 {
		if !validator.IsPhoneValid(req.Phone) {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		if req.VerifyCode == "" {
			return nil, errx.NewWithCode(errx.ParamError)
		}
	} else {
		if req.Username == "" || req.Password == "" {
			return nil, errx.NewWithCode(errx.ParamError)
		}
	}

	loginReq := userservice.LoginReq{
		Username:   req.Username,
		Password:   req.Password,
		Phone:      req.Phone,
		VerifyCode: req.VerifyCode,
		LoginType:  req.LoginType,
	}
	login, err := l.svcCtx.UserService.Login(l.ctx, &loginReq)
	if err != nil {

		return nil, err
	}
	return &types.LoginResp{
		UserId: login.UserId,
		Token:  login.Token,
	}, nil
}
