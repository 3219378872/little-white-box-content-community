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

type MarkAssistantWatchHitsReadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewMarkAssistantWatchHitsReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MarkAssistantWatchHitsReadLogic {
	return &MarkAssistantWatchHitsReadLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *MarkAssistantWatchHitsReadLogic) MarkAssistantWatchHitsRead(req *types.MarkAssistantWatchHitsReadReq) (*types.MarkAssistantWatchHitsReadResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	var hitIDs []int64
	if req != nil {
		hitIDs = req.HitIds
	}
	if _, err := l.svcCtx.AssistantService.MarkWatchHitsRead(l.ctx, &assistantservice.MarkWatchHitsReadReq{
		UserId: userID,
		HitIds: hitIDs,
	}); err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.MarkAssistantWatchHitsReadResp{}, nil
}
