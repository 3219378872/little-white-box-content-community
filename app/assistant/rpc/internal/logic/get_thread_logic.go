package logic

import (
	"context"
	"encoding/json"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetThreadLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetThreadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetThreadLogic {
	return &GetThreadLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *GetThreadLogic) GetThread(in *pb.GetThreadReq) (*pb.GetThreadResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Store == nil {
		return nil, unavailableUntilStore()
	}
	thread, err := l.svcCtx.Store.GetThread(l.ctx, in.UserId)
	if err != nil {
		return nil, err
	}
	out := &pb.AssistantThread{
		SessionId: thread.SessionID, UnreadCount: thread.UnreadCount, LastMessageId: thread.LastMessageID,
		LastMessagePreview: thread.LastMessagePreview, LastMessageAtMs: thread.LastMessageAtMs, ActiveRunId: thread.ActiveRunID,
	}
	if thread.ActiveRunID > 0 {
		if run, err := l.svcCtx.Store.GetRun(l.ctx, thread.ActiveRunID); err == nil && run != nil && !store.IsTerminalStatus(run.Status) {
			out.ActiveRunStatus = run.Status
			out.ActiveRunPhase = run.Phase
			if run.Status == store.StatusWaitingInput {
				questions, err := l.svcCtx.Store.ListQuestions(l.ctx, run.ID)
				if err != nil {
					return nil, err
				}
				for _, question := range questions {
					if question.Status == "pending" {
						raw, err := json.Marshal(question)
						if err != nil {
							return nil, err
						}
						out.QuestionRequestJson = string(raw)
						break
					}
				}
			}
		}
	}
	return &pb.GetThreadResp{Thread: out}, nil
}
