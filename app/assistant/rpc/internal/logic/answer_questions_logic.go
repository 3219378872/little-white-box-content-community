package logic

import (
	"context"
	"encoding/json"
	"esx/app/assistant/internal/runtime"
	"esx/app/assistant/internal/store"
	"esx/pkg/errx"

	"esx/app/assistant/rpc/internal/svc"
	pb "esx/kitex_gen/assistant"

	"esx/pkg/logging"
)

// AnswerQuestionsLogic 承载 AnswerQuestions 接口的业务逻辑；每个请求新建一个实例。
type AnswerQuestionsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewAnswerQuestionsLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewAnswerQuestionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AnswerQuestionsLogic {
	return &AnswerQuestionsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// AnswerQuestions 解码回答 JSON 后提交，返回更新后的追问。
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
