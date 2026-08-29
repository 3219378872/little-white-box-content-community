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

type ListAssistantMessagesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListAssistantMessagesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAssistantMessagesLogic {
	return &ListAssistantMessagesLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ListAssistantMessagesLogic) ListAssistantMessages(req *types.ListAssistantMessagesReq) (*types.ListAssistantMessagesResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	in := &assistantservice.ListMessagesReq{UserId: userID}
	if req != nil {
		in.SessionId, in.AfterId, in.Limit = req.SessionId, req.AfterId, req.Limit
	}
	result, err := l.svcCtx.AssistantService.ListMessages(l.ctx, in)
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	msgs := make([]types.AssistantMessage, 0, len(result.GetMessages()))
	for _, item := range result.GetMessages() {
		msgs = append(msgs, mapMessage(item))
	}
	return &types.ListAssistantMessagesResp{Messages: msgs}, nil
}
