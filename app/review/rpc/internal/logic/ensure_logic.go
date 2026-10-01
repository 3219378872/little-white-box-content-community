package logic

import (
	"context"

	"esx/app/review/internal/intake"
	"esx/app/review/rpc/internal/svc"
	pb "esx/kitex_gen/review"
)

type EnsureSubmittedLogic struct{ base }

func NewEnsureSubmittedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *EnsureSubmittedLogic {
	return &EnsureSubmittedLogic{newBase(ctx, svcCtx)}
}

// EnsureSubmitted 供业务方对账：送审超过 5 分钟仍未登记任务的对象幂等补建（RVW-041）。
// 与事件消费走同一入口，因此补送与正常送审只会产生一个任务。
func (l *EnsureSubmittedLogic) EnsureSubmitted(in *pb.EnsureSubmittedReq) (*pb.EnsureSubmittedResp, error) {
	sub, err := intake.Decode([]byte(in.GetSubmissionJson()))
	if err != nil {
		return nil, l.mapStoreError(err, "decode submission")
	}
	result, err := l.svcCtx.Ingester.Ingest(l.ctx, sub)
	if err != nil {
		return nil, l.mapStoreError(err, "ensure submitted")
	}
	if result.Created {
		l.Infow("review submission repaired by reconciliation")
	}
	return &pb.EnsureSubmittedResp{TaskId: result.Task.ID, Created: result.Created, Status: result.Task.Status}, nil
}
