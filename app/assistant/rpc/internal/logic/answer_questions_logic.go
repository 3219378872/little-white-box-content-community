package logic

import (
	"context"
	"encoding/json"
	"esx/app/assistant/internal/runtime"
	"esx/app/assistant/internal/store"
	"esx/pkg/errx"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type AnswerQuestionsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewAnswerQuestionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AnswerQuestionsLogic {
	return &AnswerQuestionsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *AnswerQuestionsLogic) AnswerQuestions(in *pb.AnswerQuestionsReq) (*pb.AnswerQuestionsResp, error) {
	if in == nil {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Store == nil {
		return nil, unavailableUntilStore()
	}
	var answers []store.QuestionAnswer
	if err := json.Unmarshal([]byte(in.AnswersJson), &answers); err != nil {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	question, err := runtime.AnswerQuestions(l.ctx, l.svcCtx.Store, l.svcCtx.Notify, in.UserId, in.RunId, in.QuestionRequestId, in.RequestId, answers)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(question)
	if err != nil {
		return nil, err
	}
	return &pb.AnswerQuestionsResp{QuestionRequestJson: string(raw)}, nil
}
