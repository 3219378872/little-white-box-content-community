// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package login

import (
	"context"
	gatewaymiddleware "esx/app/gateway/internal/middleware"
	"esx/app/user/rpc/userservice"
	"esx/pkg/validator"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SendVerifyCodeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 发送验证码
func NewSendVerifyCodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SendVerifyCodeLogic {
	return &SendVerifyCodeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SendVerifyCodeLogic) SendVerifyCode(req *types.SendVerifyCodeReq) (resp *types.SendVerifyCodeResp, err error) {
	// 校验手机号是否合法
	err = validator.ValidatePhone(req.Phone)
	if err != nil {
		return nil, err
	}

	_, err = l.svcCtx.UserService.SendVerifyCode(l.ctx, &userservice.SendVerifyCodeReq{
		Phone:    req.Phone,
		Type:     req.Type,
		ClientIp: gatewaymiddleware.ClientIPFromContext(l.ctx),
	})

	if err != nil {
		return nil, err
	}
	return
}
