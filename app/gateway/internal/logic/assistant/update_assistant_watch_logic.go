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

type UpdateAssistantWatchLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateAssistantWatchLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateAssistantWatchLogic {
	return &UpdateAssistantWatchLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *UpdateAssistantWatchLogic) UpdateAssistantWatch(req *types.UpdateAssistantWatchReq) (*types.UpdateAssistantWatchResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	if req == nil || req.Id <= 0 || req.ExpectedVersion <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	result, err := l.svcCtx.AssistantService.UpdateWatchTask(l.ctx, &assistantservice.UpdateWatchTaskReq{
		UserId:          userID,
		Id:              req.Id,
		Enabled:         req.Enabled,
		ExpectedVersion: req.ExpectedVersion,
	})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.UpdateAssistantWatchResp{Task: mapWatch(result.GetTask())}, nil
}
