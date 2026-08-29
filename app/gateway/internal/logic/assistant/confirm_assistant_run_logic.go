package assistant

import (
	"context"
	"strings"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ConfirmAssistantRunLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewConfirmAssistantRunLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ConfirmAssistantRunLogic {
	return &ConfirmAssistantRunLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ConfirmAssistantRunLogic) ConfirmAssistantRun(req *types.ConfirmAssistantRunReq) (*types.ConfirmAssistantRunResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	if req == nil || req.Id <= 0 || strings.TrimSpace(req.CallId) == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if _, err := l.svcCtx.AssistantService.ConfirmRunTool(l.ctx, &assistantservice.ConfirmRunToolReq{
		UserId: userID, RunId: req.Id, CallId: req.CallId, Approved: req.Approved,
	}); err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.ConfirmAssistantRunResp{}, nil
}
