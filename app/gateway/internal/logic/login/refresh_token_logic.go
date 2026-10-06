package login

import (
	"context"

	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"

	"esx/pkg/logging"
)

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
