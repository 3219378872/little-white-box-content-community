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

type MarkAssistantThreadReadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewMarkAssistantThreadReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MarkAssistantThreadReadLogic {
	return &MarkAssistantThreadReadLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *MarkAssistantThreadReadLogic) MarkAssistantThreadRead() (*types.MarkAssistantThreadReadResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	result, err := l.svcCtx.AssistantService.MarkThreadRead(l.ctx, &assistantservice.MarkThreadReadReq{UserId: userID})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.MarkAssistantThreadReadResp{UnreadCount: result.GetUnreadCount()}, nil
}
