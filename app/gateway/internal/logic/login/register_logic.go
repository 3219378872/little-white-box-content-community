// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package login

import (
	"context"
	"errx"
	"esx/pkg/validator"
	"gateway/internal/svc"
	"gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type RegisterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 用户注册
func NewRegisterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterLogic {
	return &RegisterLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RegisterLogic) Register(req *types.RegisterReq) (resp *types.RegisterResp, err error) {
	if req.Username != "" {
		// 用户名注册，校验用户名长度
		err = validator.ValidateUserName(req.Username)
		if err != nil {
			return nil, err
		}
		// 校验密码强度
		_, err = validator.CheckPasswordStrength(req.Password)
		if err != nil {
			return nil, err
		}
	}

	if req.Phone != "" {
		// 手机号注册，校验手机号是否合法
		err = validator.ValidatePhone(req.Phone)
		if err != nil {
			return nil, err
		}
	}

	if req.Username == "" && req.Phone == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	register, err := l.svcCtx.UserService.Register(
		l.ctx,
		RegisterReqConvert(req),
	)
	if err != nil {
		return nil, err
	}

	return RegisterRespConvert(register), nil
}
