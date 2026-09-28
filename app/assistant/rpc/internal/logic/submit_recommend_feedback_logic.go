package logic

import (
	"context"
	"strings"

	"esx/app/assistant/rpc/internal/svc"
	pb "esx/kitex_gen/assistant"
	"esx/pkg/errx"

	logx "esx/pkg/logging"
)

type SubmitRecommendFeedbackLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSubmitRecommendFeedbackLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SubmitRecommendFeedbackLogic {
	return &SubmitRecommendFeedbackLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *SubmitRecommendFeedbackLogic) SubmitRecommendFeedback(in *pb.SubmitRecommendFeedbackReq) (*pb.SubmitRecommendFeedbackResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(in.Reason)
	if in.PostId <= 0 || reason == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if l.svcCtx == nil || l.svcCtx.Memory == nil {
		return nil, unavailableUntilStore()
	}
	if err := l.svcCtx.Memory.RecordFeedback(l.ctx, in.UserId, in.RequestId, in.PostId, reason); err != nil {
		return nil, err
	}
	return &pb.SubmitRecommendFeedbackResp{}, nil
}
