package assistant

import (
	"context"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetAssistantThreadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetAssistantThreadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAssistantThreadLogic {
	return &GetAssistantThreadLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *GetAssistantThreadLogic) GetAssistantThread() (*types.GetAssistantThreadResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	result, err := l.svcCtx.AssistantService.GetThread(l.ctx, &assistantservice.GetThreadReq{UserId: userID})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.GetAssistantThreadResp{Thread: mapThread(result.GetThread())}, nil
}
