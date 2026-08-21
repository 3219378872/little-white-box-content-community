// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package login

import (
	"context"

	"errx"
	"user/userservice"

	"gateway/internal/svc"
	"gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type RefreshTokenLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 刷新令牌：凭 refresh token 换取全新令牌对
func NewRefreshTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RefreshTokenLogic {
	return &RefreshTokenLogic{
		Logger: logx.WithContext(ctx),
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
