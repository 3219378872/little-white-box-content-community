// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package login

import (
	"context"
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
	register, err := l.svcCtx.UserService.Register(
		l.ctx,
		RegisterReqConvert(req),
	)
	if err != nil {
		return nil, err
	}

	return RegisterRespConvert(register), nil
}
