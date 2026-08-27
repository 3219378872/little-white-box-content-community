package logic

import (
	"context"
	"strconv"
	"strings"
	"time"

	"esx/app/assistant/rpc/internal/memory"
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
	if reason == "not_interested" {
		_ = l.svcCtx.Memory.Apply(l.ctx, in.UserId, memory.Candidate{
			Layer: memory.LayerProfile, Dimension: "post", Value: strconv.FormatInt(in.PostId, 10),
			Score: -0.6, Source: memory.SourceExplicit, Confidence: 0.8,
		}, time.Now())
	}
	return &pb.SubmitRecommendFeedbackResp{}, nil
}
