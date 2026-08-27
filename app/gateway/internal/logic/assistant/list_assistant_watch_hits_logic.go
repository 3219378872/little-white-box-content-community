package assistant

import (
	"context"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListAssistantWatchHitsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListAssistantWatchHitsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAssistantWatchHitsLogic {
	return &ListAssistantWatchHitsLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ListAssistantWatchHitsLogic) ListAssistantWatchHits(req *types.ListAssistantWatchHitsReq) (*types.ListAssistantWatchHitsResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	unreadOnly := req != nil && req.UnreadOnly
	result, err := l.svcCtx.AssistantService.ListWatchHits(l.ctx, &assistantservice.ListWatchHitsReq{
		UserId:     userID,
		UnreadOnly: unreadOnly,
	})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	hits := make([]types.AssistantWatchHit, 0, len(result.GetHits()))
	for _, hit := range result.GetHits() {
		if hit == nil {
			continue
		}
		hits = append(hits, types.AssistantWatchHit{
			Id:        hit.Id,
			TaskId:    hit.TaskId,
			PostId:    hit.PostId,
			Title:     hit.Title,
			Summary:   hit.Summary,
			CreatedAt: hit.CreatedAt,
			Read:      hit.Read,
		})
	}
	return &types.ListAssistantWatchHitsResp{Hits: hits}, nil
}
