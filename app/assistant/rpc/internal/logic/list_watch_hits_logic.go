package logic

import (
	"context"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

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
		return &pb.ListWatchHitsResp{Hits: []*pb.WatchHit{}}, nil
	}
	hits, err := l.svcCtx.Watch.ListHits(l.ctx, in.UserId, in.UnreadOnly)
	if err != nil {
		return nil, err
	}
	out := make([]*pb.WatchHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, &pb.WatchHit{
			Id: hit.ID, TaskId: hit.TaskID, PostId: hit.PostID, Title: hit.Title,
			Summary: hit.Summary, CreatedAt: hit.CreatedAt, Read: hit.Read,
		})
	}
	return &pb.ListWatchHitsResp{Hits: out}, nil
}
