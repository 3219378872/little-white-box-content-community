package logic

import (
	"context"

	"esx/app/assistant/internal/runtime"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type PostMessageLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewPostMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PostMessageLogic {
	return &PostMessageLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *PostMessageLogic) PostMessage(in *pb.PostMessageReq) (*pb.PostMessageResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Acceptor == nil || l.svcCtx.Store == nil {
		return nil, unavailableUntilStore()
	}
	consentOK := false
	var consentVersion int32
	if l.svcCtx.UserService != nil {
		consent, err := l.svcCtx.UserService.GetAgentCapabilityConsent(l.ctx, &userservice.GetAgentCapabilityConsentReq{UserId: in.UserId})
		if err != nil {
			return nil, errx.FromRPCError(err)
		}
		consentOK = consent != nil && consent.Granted && consent.ConsentVersion > 0
		if consent != nil {
			consentVersion = consent.ConsentVersion
		}
	}
	attachments := make([]runtime.Attachment, 0, len(in.Attachments))
	for _, item := range in.Attachments {
		if item != nil {
			attachments = append(attachments, runtime.Attachment{MediaID: item.MediaId, URL: item.Url})
		}
	}
	result, err := l.svcCtx.Acceptor.Accept(l.ctx, runtime.AcceptInput{
		UserID: in.UserId, Message: in.Message, RequestID: in.RequestId,
		Attachments: attachments, ContextPostID: in.ContextPostId, ConsentOK: consentOK, ConsentVersion: consentVersion,
	})
	if err != nil {
		return nil, err
	}
	return &pb.PostMessageResp{MessageId: result.MessageID, SessionId: result.SessionID, RunId: result.RunID, Disposition: result.Disposition}, nil
}
