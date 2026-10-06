package login

import (
	"context"
	"esx/app/gateway/internal/logic/rpcx"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"
	"esx/pkg/validator"

	"esx/pkg/logging"
)

// LoginLogic 承载 Login 接口的业务逻辑；每个请求新建一个实例。
type LoginLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 用户登录
func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Login 按登录方式做入参校验后转交用户服务签发令牌。
func (l *LoginLogic) Login(req *types.LoginReq) (resp *types.LoginResp, err error) {
	// LoginType 2 为手机号验证码登录，其余按用户名密码登录。
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
