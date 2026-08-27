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

type SubmitAssistantRecommendFeedbackLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSubmitAssistantRecommendFeedbackLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SubmitAssistantRecommendFeedbackLogic {
	return &SubmitAssistantRecommendFeedbackLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *SubmitAssistantRecommendFeedbackLogic) SubmitAssistantRecommendFeedback(req *types.AssistantRecommendFeedbackReq) (*types.AssistantRecommendFeedbackResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
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
