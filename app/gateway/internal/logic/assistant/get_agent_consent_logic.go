package assistant

import (
	"context"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

// GetAgentConsentLogic 承载 GetAgentConsent 接口的业务逻辑；每个请求新建一个实例。
type GetAgentConsentLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 查询 Agent 能力授权状态（AGNT-004）
func NewGetAgentConsentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAgentConsentLogic {
	return &GetAgentConsentLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetAgentConsent 返回用户对助手能力的授权状态及当前协议版本，供前端判断是否需要重新授权。
func (l *GetAgentConsentLogic) GetAgentConsent() (resp *types.GetAgentConsentResp, err error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	result, err := l.svcCtx.UserService.GetAgentCapabilityConsent(l.ctx, &userservice.GetAgentCapabilityConsentReq{
		UserId: userID,
	})
	if err != nil {
		l.Errorw("UserService.GetAgentCapabilityConsent RPC failed",
			logging.Field("userId", userID),
			logging.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}
	if result == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &types.GetAgentConsentResp{
		Granted:        result.Granted,
		GrantedAt:      result.GrantedAt,
		RevokedAt:      result.RevokedAt,
		ConsentVersion: result.ConsentVersion,
		CurrentVersion: result.CurrentVersion,
	}, nil
}
