package logic

import (
	"context"
	"strings"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type SubmitRecommendFeedbackLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSubmitRecommendFeedbackLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SubmitRecommendFeedbackLogic {
	return &SubmitRecommendFeedbackLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *SubmitRecommendFeedbackLogic) SubmitRecommendFeedback(in *pb.SubmitRecommendFeedbackReq) (*pb.SubmitRecommendFeedbackResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if in.PostId <= 0 || strings.TrimSpace(in.Reason) == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	return nil, unavailableUntilStore()
}
