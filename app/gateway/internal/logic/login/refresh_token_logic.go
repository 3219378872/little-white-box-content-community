package login

import (
	"context"

	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"

	"esx/pkg/logging"
)

// RefreshTokenLogic 承载 RefreshToken 接口的业务逻辑；每个请求新建一个实例。
type RefreshTokenLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 刷新令牌：凭 refresh token 换取全新令牌对
func NewRefreshTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RefreshTokenLogic {
	return &RefreshTokenLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// RefreshToken 用刷新令牌换取新的令牌对；令牌校验与轮换由用户服务负责。
func (l *RefreshTokenLogic) RefreshToken(req *types.RefreshTokenReq) (resp *types.RefreshTokenResp, err error) {
	if req.RefreshToken == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	rpcResp, err := l.svcCtx.UserService.RefreshToken(l.ctx, &userservice.RefreshTokenReq{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.RefreshTokenResp{
		Token:        rpcResp.Token,
		RefreshToken: rpcResp.RefreshToken,
	}, nil
}
