// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package login

import (
	"context"
	"esx/app/gateway/internal/logic/rpcx"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"
	"esx/pkg/validator"

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
		return nil, rpcx.Error(l.Logger, "UserService.Login", err)
	}
	return &types.LoginResp{
		UserId:       login.UserId,
		Token:        login.Token,
		RefreshToken: login.RefreshToken,
	}, nil
}
