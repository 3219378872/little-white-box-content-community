package assistant

import (
	"context"
	"strings"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

// SubmitAssistantRecommendFeedbackLogic 承载 SubmitAssistantRecommendFeedback 接口的业务逻辑；每个请求新建一个实例。
type SubmitAssistantRecommendFeedbackLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewSubmitAssistantRecommendFeedbackLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewSubmitAssistantRecommendFeedbackLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SubmitAssistantRecommendFeedbackLogic {
	return &SubmitAssistantRecommendFeedbackLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// SubmitAssistantRecommendFeedback 记录用户对助手推荐帖子的反馈原因。
func (l *SubmitAssistantRecommendFeedbackLogic) SubmitAssistantRecommendFeedback(req *types.AssistantRecommendFeedbackReq) (*types.AssistantRecommendFeedbackResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.PostId <= 0 || strings.TrimSpace(req.Reason) == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if _, err := l.svcCtx.AssistantService.SubmitRecommendFeedback(l.ctx, &assistantservice.SubmitRecommendFeedbackReq{
		UserId:    userID,
		RequestId: req.RequestId,
		PostId:    req.PostId,
		Reason:    req.Reason,
	}); err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.AssistantRecommendFeedbackResp{}, nil
}
