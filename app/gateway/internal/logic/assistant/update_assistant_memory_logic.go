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

type UpdateAssistantMemoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateAssistantMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateAssistantMemoryLogic {
	return &UpdateAssistantMemoryLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *UpdateAssistantMemoryLogic) UpdateAssistantMemory(req *types.UpdateAssistantMemoryReq) (*types.UpdateAssistantMemoryResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	if req == nil || req.Id <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if _, err := l.svcCtx.AssistantService.UpdateMemory(l.ctx, &assistantservice.UpdateMemoryReq{
		UserId:     userID,
		Id:         req.Id,
		Value:      req.Value,
		Score:      req.Score,
		Suppressed: req.Suppressed,
	}); err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.UpdateAssistantMemoryResp{}, nil
}
