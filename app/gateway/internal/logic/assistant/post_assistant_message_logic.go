package assistant

import (
	"context"
	"strings"
	"unicode/utf8"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type PostAssistantMessageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPostAssistantMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PostAssistantMessageLogic {
	return &PostAssistantMessageLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *PostAssistantMessageLogic) PostAssistantMessage(req *types.PostAssistantMessageReq) (*types.PostAssistantMessageResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	if req == nil || strings.TrimSpace(req.Message) == "" || utf8.RuneCountInString(strings.TrimSpace(req.Message)) > 2000 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	attachments := make([]*assistantservice.Attachment, 0, len(req.Attachments))
	for _, item := range req.Attachments {
		attachments = append(attachments, &assistantservice.Attachment{MediaId: item.MediaId, Url: item.Url})
	}
	result, err := l.svcCtx.AssistantService.PostMessage(l.ctx, &assistantservice.PostMessageReq{
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
