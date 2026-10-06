package assistant

import (
	"context"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

type CancelAssistantRunLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCancelAssistantRunLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CancelAssistantRunLogic {
	return &CancelAssistantRunLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *CancelAssistantRunLogic) CancelAssistantRun(req *types.CancelAssistantRunReq) (*types.CancelAssistantRunResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.Id <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if _, err := l.svcCtx.AssistantService.CancelRun(l.ctx, &assistantservice.CancelRunReq{UserId: userID, RunId: req.Id}); err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.CancelAssistantRunResp{}, nil
}
