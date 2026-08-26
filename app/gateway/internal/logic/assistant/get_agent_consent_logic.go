// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package assistant

import (
	"context"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetAgentConsentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 查询 Agent 能力授权状态（AGNT-004）
func NewGetAgentConsentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAgentConsentLogic {
	return &GetAgentConsentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetAgentConsentLogic) GetAgentConsent() (resp *types.GetAgentConsentResp, err error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	result, err := l.svcCtx.UserService.GetAgentCapabilityConsent(l.ctx, &userservice.GetAgentCapabilityConsentReq{
		UserId: userID,
	})
	if err != nil {
		l.Errorw("UserService.GetAgentCapabilityConsent RPC failed",
			logx.Field("userId", userID),
			logx.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}
	if result == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &types.GetAgentConsentResp{
		Granted:   result.Granted,
		GrantedAt: result.GrantedAt,
		RevokedAt: result.RevokedAt,
	}, nil
}
