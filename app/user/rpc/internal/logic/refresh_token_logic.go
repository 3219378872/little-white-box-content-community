package logic

import (
	"context"
	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type RefreshTokenLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRefreshTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RefreshTokenLogic {
	return &RefreshTokenLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// RefreshToken 校验并轮换刷新令牌：旧 refresh token 一次性失效，
// 返回全新的访问/刷新令牌对。重放已轮换的令牌会被拒绝。
func (l *RefreshTokenLogic) RefreshToken(in *pb.RefreshTokenReq) (*pb.RefreshTokenResp, error) {
	if in.GetRefreshToken() == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	access, refresh, err := rotateRefreshToken(l.ctx, l.svcCtx, in.GetRefreshToken())
	if err != nil {
		return nil, err
	}
	return &pb.RefreshTokenResp{
		Token:        access,
		RefreshToken: refresh,
	}, nil
}
