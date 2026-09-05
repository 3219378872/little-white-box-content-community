package logic

import (
	"context"
	"encoding/json"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/pkg/errx"

	"esx/app/assistant/internal/runtime"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type SubscribeRunEventsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSubscribeRunEventsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SubscribeRunEventsLogic {
	return &SubscribeRunEventsLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *SubscribeRunEventsLogic) SubscribeRunEvents(in *pb.SubscribeRunEventsReq, stream pb.AssistantService_SubscribeRunEventsServer) error {
	if in == nil {
		return requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return err
	}
	if l.svcCtx == nil || l.svcCtx.Store == nil {
		return unavailableUntilStore()
	}
	ctx := l.ctx
	if stream != nil {
		ctx = stream.Context()
	}
	return runtime.Subscribe(ctx, l.svcCtx.Store, l.svcCtx.Notify, in.UserId, in.RunId, in.AfterSeq, func(ev *pb.RunEvent) error {
		if ev.AnswerPresentationJson != "" {
			var answer store.AnswerPresentation
			if err := json.Unmarshal([]byte(ev.AnswerPresentationJson), &answer); err != nil {
				return err
			}
			msg, err := l.svcCtx.Store.GetMessage(ctx, in.UserId, answer.MessageID)
			if err != nil || msg.DeletedAtMs > 0 {
				return errx.NewWithCode(errx.NotFound)
			}
			tool.RevalidatePresentation(ctx, tool.Clients{Content: l.svcCtx.ContentService, Store: l.svcCtx.Store}, in.UserId, &answer)
			raw, err := json.Marshal(answer)
			if err != nil {
				return err
			}
			ev.AnswerPresentationJson = string(raw)
		}
		return stream.Send(ev)
	})
}
