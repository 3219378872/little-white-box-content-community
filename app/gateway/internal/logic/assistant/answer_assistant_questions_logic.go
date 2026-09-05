// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package assistant

import (
	"context"
	"encoding/json"
	"esx/app/assistant/rpc/assistantservice"
	"esx/pkg/errx"
	"esx/pkg/jwtx"
	"strings"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AnswerAssistantQuestionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAnswerAssistantQuestionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AnswerAssistantQuestionsLogic {
	return &AnswerAssistantQuestionsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AnswerAssistantQuestionsLogic) AnswerAssistantQuestions(req *types.AnswerAssistantQuestionsReq) (resp *types.AnswerAssistantQuestionsResp, err error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
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
	question := decodeResearch[types.AssistantQuestionRequest](result.GetQuestionRequestJson())
	if question == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &types.AnswerAssistantQuestionsResp{QuestionRequest: *question}, nil
}
