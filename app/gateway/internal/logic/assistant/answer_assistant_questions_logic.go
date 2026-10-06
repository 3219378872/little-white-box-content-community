package assistant

import (
	"context"
	"encoding/json"
	"esx/app/assistant/rpc/assistantservice"
	"esx/pkg/errx"
	"strings"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

// AnswerAssistantQuestionsLogic 承载 AnswerAssistantQuestions 接口的业务逻辑；每个请求新建一个实例。
type AnswerAssistantQuestionsLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewAnswerAssistantQuestionsLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewAnswerAssistantQuestionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AnswerAssistantQuestionsLogic {
	return &AnswerAssistantQuestionsLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// AnswerAssistantQuestions 提交对助手追问的 1–3 个回答，并返回服务端更新后的追问状态。
func (l *AnswerAssistantQuestionsLogic) AnswerAssistantQuestions(req *types.AnswerAssistantQuestionsReq) (resp *types.AnswerAssistantQuestionsResp, err error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.Id <= 0 || strings.TrimSpace(req.QuestionRequestId) == "" || strings.TrimSpace(req.RequestId) == "" || len(req.RequestId) > 64 || len(req.Answers) < 1 || len(req.Answers) > 3 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	raw, err := json.Marshal(req.Answers)
	if err != nil {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	result, err := l.svcCtx.AssistantService.AnswerQuestions(l.ctx, &assistantservice.AnswerQuestionsReq{UserId: userID, RunId: req.Id, QuestionRequestId: req.QuestionRequestId, RequestId: req.RequestId, AnswersJson: string(raw)})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	// 追问状态是网关返回体的必填字段，无法解码说明服务端数据损坏。
	question := decodeResearch[types.AssistantQuestionRequest](result.GetQuestionRequestJson())
	if question == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &types.AnswerAssistantQuestionsResp{QuestionRequest: *question}, nil
}
