package assistant

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

// PostAssistantMessageLogic 承载 PostAssistantMessage 接口的业务逻辑；每个请求新建一个实例。
type PostAssistantMessageLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewPostAssistantMessageLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewPostAssistantMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PostAssistantMessageLogic {
	return &PostAssistantMessageLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// PostAssistantMessage 发送一条用户消息并触发助手运行；正文去空白后限 2000 字符。
func (l *PostAssistantMessageLogic) PostAssistantMessage(req *types.PostAssistantMessageReq) (*types.PostAssistantMessageResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || strings.TrimSpace(req.Message) == "" || utf8.RuneCountInString(strings.TrimSpace(req.Message)) > 2000 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	attachments := make([]*assistantservice.Attachment, 0, len(req.Attachments))
	for _, item := range req.Attachments {
		attachments = append(attachments, &assistantservice.Attachment{MediaId: item.MediaId, Url: item.Url})
	}
	// 追问上下文以 JSON 字符串透传，网关不解释其结构。
	var questionContext string
	if req.QuestionContext != nil {
		raw, err := json.Marshal(req.QuestionContext)
		if err != nil {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		questionContext = string(raw)
	}
	result, err := l.svcCtx.AssistantService.PostMessage(l.ctx, &assistantservice.PostMessageReq{
		ClientProtocolVersion: req.ClientProtocolVersion, QuestionContextJson: questionContext,
		UserId: userID, Message: strings.TrimSpace(req.Message), RequestId: req.RequestId,
		Attachments: attachments, ContextPostId: req.ContextPostId,
	})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.PostAssistantMessageResp{
		MessageId: result.GetMessageId(), SessionId: result.GetSessionId(), RunId: result.GetRunId(), Disposition: result.GetDisposition(),
	}, nil
}
