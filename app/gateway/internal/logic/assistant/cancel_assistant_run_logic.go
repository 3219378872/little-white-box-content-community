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

// CancelAssistantRunLogic 承载 CancelAssistantRun 接口的业务逻辑；每个请求新建一个实例。
type CancelAssistantRunLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewCancelAssistantRunLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewCancelAssistantRunLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CancelAssistantRunLogic {
	return &CancelAssistantRunLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// CancelAssistantRun 请求取消当前用户的一次运行。
func (l *CancelAssistantRunLogic) CancelAssistantRun(req *types.CancelAssistantRunReq) (*types.CancelAssistantRunResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.Id <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if _, err := l.svcCtx.AssistantService.CancelRun(l.ctx, &assistantservice.CancelRunReq{UserId: userID, RunId: req.Id}); err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.CancelAssistantRunResp{}, nil
}
