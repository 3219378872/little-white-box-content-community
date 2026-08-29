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

type UndoAssistantMemoryChangeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUndoAssistantMemoryChangeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UndoAssistantMemoryChangeLogic {
	return &UndoAssistantMemoryChangeLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *UndoAssistantMemoryChangeLogic) UndoAssistantMemoryChange(req *types.UndoAssistantMemoryChangeReq) (*types.UndoAssistantMemoryChangeResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	if req == nil || req.Id <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	result, err := l.svcCtx.AssistantService.UndoMemoryChange(l.ctx, &assistantservice.UndoMemoryChangeReq{UserId: userID, ChangeId: req.Id})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.UndoAssistantMemoryChangeResp{Entry: mapMemory(result.GetEntry())}, nil
}
