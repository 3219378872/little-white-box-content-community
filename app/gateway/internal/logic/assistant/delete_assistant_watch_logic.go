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

type DeleteAssistantWatchLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteAssistantWatchLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteAssistantWatchLogic {
	return &DeleteAssistantWatchLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *DeleteAssistantWatchLogic) DeleteAssistantWatch(req *types.DeleteAssistantWatchReq) (*types.DeleteAssistantWatchResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	if req == nil || req.Id <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if _, err := l.svcCtx.AssistantService.DeleteWatchTask(l.ctx, &assistantservice.DeleteWatchTaskReq{
		UserId: userID,
		Id:     req.Id,
	}); err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.DeleteAssistantWatchResp{}, nil
}
