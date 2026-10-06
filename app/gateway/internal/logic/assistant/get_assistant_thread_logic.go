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

type GetAssistantThreadLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetAssistantThreadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAssistantThreadLogic {
	return &GetAssistantThreadLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *GetAssistantThreadLogic) GetAssistantThread() (*types.GetAssistantThreadResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	result, err := l.svcCtx.AssistantService.GetThread(l.ctx, &assistantservice.GetThreadReq{UserId: userID})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.GetAssistantThreadResp{Thread: mapThread(result.GetThread())}, nil
}
