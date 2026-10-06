package logic

import (
	"context"
	"strings"

	"esx/app/assistant/rpc/internal/svc"
	pb "esx/kitex_gen/assistant"
	"esx/pkg/errx"

	"esx/pkg/logging"
)

// SubmitRecommendFeedbackLogic 承载 SubmitRecommendFeedback 接口的业务逻辑；每个请求新建一个实例。
type SubmitRecommendFeedbackLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewSubmitRecommendFeedbackLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewSubmitRecommendFeedbackLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SubmitRecommendFeedbackLogic {
	return &SubmitRecommendFeedbackLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// SubmitRecommendFeedback 记录用户对助手推荐帖子的反馈原因。
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
