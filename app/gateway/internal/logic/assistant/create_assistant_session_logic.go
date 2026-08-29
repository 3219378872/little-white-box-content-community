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

type CreateAssistantSessionLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateAssistantSessionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateAssistantSessionLogic {
	return &CreateAssistantSessionLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *CreateAssistantSessionLogic) CreateAssistantSession() (*types.CreateAssistantSessionResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	result, err := l.svcCtx.AssistantService.CreateSession(l.ctx, &assistantservice.CreateSessionReq{UserId: userID})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.CreateAssistantSessionResp{SessionId: result.GetSessionId()}, nil
}
