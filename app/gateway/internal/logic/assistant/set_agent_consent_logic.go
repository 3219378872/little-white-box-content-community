// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package assistant

import (
	"context"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type SetAgentConsentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 记录或撤销 Agent 能力授权（AGNT-004/006）
func NewSetAgentConsentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetAgentConsentLogic {
	return &SetAgentConsentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SetAgentConsentLogic) SetAgentConsent(req *types.SetAgentConsentReq) (resp *types.SetAgentConsentResp, err error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	if _, err := l.svcCtx.UserService.SetAgentCapabilityConsent(l.ctx, &userservice.SetAgentCapabilityConsentReq{
		UserId:  userID,
		Granted: req.Granted,
	}); err != nil {
		l.Errorw("UserService.SetAgentCapabilityConsent RPC failed",
			logx.Field("userId", userID),
			logx.Field("granted", req.Granted),
			logx.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}
	if !req.Granted {
		if _, err := l.svcCtx.AssistantService.RevokeConsent(l.ctx, &assistantservice.RevokeConsentReq{UserId: userID}); err != nil {
			l.Errorw("AssistantService.RevokeConsent RPC failed", logx.Field("userId", userID), logx.Field("err", err.Error()))
			return nil, errx.FromRPCError(err)
		}
	}
	return &types.SetAgentConsentResp{}, nil
}
