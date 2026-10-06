package logic

import (
	"context"
	"esx/app/user/rpc/internal/svc"
	pb "esx/kitex_gen/user"

	"esx/pkg/errx"

	"esx/pkg/logging"
)

// RefreshTokenLogic 承载 RefreshToken 接口的业务逻辑；每个请求新建一个实例。
type RefreshTokenLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewRefreshTokenLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewRefreshTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RefreshTokenLogic {
	return &RefreshTokenLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
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
