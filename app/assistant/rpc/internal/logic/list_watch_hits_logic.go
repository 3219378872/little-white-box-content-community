package logic

import (
	"context"
	"strings"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/internal/tool"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/app/assistant/watch"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListWatchHitsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewListWatchHitsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWatchHitsLogic {
	return &ListWatchHitsLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ListWatchHitsLogic) ListWatchHits(in *pb.ListWatchHitsReq) (*pb.ListWatchHitsResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Watch == nil {
		return nil, unavailableUntilStore()
	}
	hits, err := l.svcCtx.Watch.ListHits(l.ctx, in.UserId, in.UnreadOnly)
	if err != nil {
		return nil, err
	}
	hits = redactUnavailableWatchHits(l.ctx, l.svcCtx, hits)
	out := make([]*pb.WatchHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, &pb.WatchHit{
			Id: hit.ID, TaskId: hit.TaskID, PostId: hit.PostID, Title: hit.Title,
			Summary: hit.Summary, CreatedAt: hit.CreatedAt, Read: hit.Read,
		})
	}
	return &pb.ListWatchHitsResp{Hits: out}, nil
}

func redactUnavailableWatchHits(ctx context.Context, svcCtx *svc.ServiceContext, hits []watch.Hit) []watch.Hit {
	if len(hits) == 0 {
		return hits
	}
	if svcCtx == nil || svcCtx.ContentService == nil {
		return redactAllWatchHitContent(hits)
	}
	ids := make([]int64, 0, len(hits))
	for _, hit := range hits {
		if hit.PostID > 0 {
			ids = append(ids, hit.PostID)
		}
	}
	if len(ids) == 0 {
		return hits
	}
	published, err := tool.PublishedPosts(ctx, svcCtx.ContentService, ids)
	if err != nil {
		logx.WithContext(ctx).Infow("watch hit visibility backfill failed", logx.Field("err", err.Error()))
		return redactAllWatchHitContent(hits)
	}
	out := make([]watch.Hit, len(hits))
	copy(out, hits)
	for i := range out {
		if out[i].PostID <= 0 {
			continue
		}
		info := published[out[i].PostID]
		if info == nil {
			out[i].Title = ""
			out[i].Summary = ""
			continue
		}
		if strings.TrimSpace(out[i].Title) == "" {
			out[i].Title = info.Title
		}
	}
	return out
}

func redactAllWatchHitContent(hits []watch.Hit) []watch.Hit {
	out := append([]watch.Hit(nil), hits...)
	for i := range out {
		if out[i].PostID > 0 {
			out[i].Title = ""
			out[i].Summary = ""
		}
	}
	return out
}
