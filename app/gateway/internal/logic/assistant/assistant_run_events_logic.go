package assistant

import (
	"context"
	"io"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AssistantRunEventsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssistantRunEventsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssistantRunEventsLogic {
	return &AssistantRunEventsLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *AssistantRunEventsLogic) AssistantRunEvents(req *types.AssistantRunEventsReq, client chan<- *types.AssistantRunEvent) error {
	if client == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil || userID <= 0 {
		return errx.NewWithCode(errx.LoginRequired)
	}
	if req == nil || req.Id <= 0 {
		return errx.NewWithCode(errx.ParamError)
	}
	if l.svcCtx == nil || l.svcCtx.AssistantService == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	stream, err := l.svcCtx.AssistantService.SubscribeRunEvents(l.ctx, &assistantservice.SubscribeRunEventsReq{
		UserId: userID, RunId: req.Id, AfterSeq: req.AfterSeq,
	})
	if err != nil {
		return errx.FromRPCError(err)
	}
	for {
		event, recvErr := stream.Recv()
		if recvErr == io.EOF {
			return nil
		}
		if recvErr != nil {
			if l.ctx.Err() != nil {
				return nil
			}
			if status.Code(recvErr) == codes.Canceled {
				return nil
			}
			return errx.FromRPCError(recvErr)
		}
		mapped := mapRunEvent(event)
		if mapped == nil {
			continue
		}
		select {
		case <-l.ctx.Done():
			return nil
		case client <- mapped:
		}
	}
}
