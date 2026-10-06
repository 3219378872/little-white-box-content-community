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

type MarkAssistantThreadReadLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewMarkAssistantThreadReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MarkAssistantThreadReadLogic {
	return &MarkAssistantThreadReadLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *MarkAssistantThreadReadLogic) MarkAssistantThreadRead() (*types.MarkAssistantThreadReadResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	result, err := l.svcCtx.AssistantService.MarkThreadRead(l.ctx, &assistantservice.MarkThreadReadReq{UserId: userID})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.MarkAssistantThreadReadResp{UnreadCount: result.GetUnreadCount()}, nil
}
