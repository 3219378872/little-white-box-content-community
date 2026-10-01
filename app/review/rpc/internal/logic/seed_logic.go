package logic

import (
	"context"

	"esx/app/review/internal/store"
	"esx/app/review/rpc/internal/svc"
	pb "esx/kitex_gen/review"
	"esx/pkg/errx"
)

type ListSeedsLogic struct{ base }

func NewListSeedsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListSeedsLogic {
	return &ListSeedsLogic{newBase(ctx, svcCtx)}
}

func (l *ListSeedsLogic) ListSeeds(in *pb.ListSeedsReq) (*pb.ListSeedsResp, error) {
	if _, err := l.reviewer(in.GetUserId(), store.RolePolicyAdmin); err != nil {
		return nil, err
	}
	status := in.GetStatus()
	if status == "" {
		status = store.SeedCandidate
	}
	if status != store.SeedCandidate && status != store.SeedActive && status != store.SeedRetired {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	seeds, err := l.svcCtx.Store.ListSeeds(l.ctx, status, int(in.GetLimit()))
	if err != nil {
		return nil, l.mapStoreError(err, "list seeds")
	}
	resp := &pb.ListSeedsResp{}
	for i := range seeds {
		resp.Seeds = append(resp.Seeds, seedView(&seeds[i]))
	}
	return resp, nil
}

type ConfirmSeedLogic struct{ base }

func NewConfirmSeedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ConfirmSeedLogic {
	return &ConfirmSeedLogic{newBase(ctx, svcCtx)}
}

// ConfirmSeed 需要政策管理权限且不同于提名人（RVW-030）。
func (l *ConfirmSeedLogic) ConfirmSeed(in *pb.SeedActionReq) (*pb.SeedResp, error) {
	return transitionSeed(l.base, in, store.SeedActive)
}

type RetireSeedLogic struct{ base }

func NewRetireSeedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RetireSeedLogic {
	return &RetireSeedLogic{newBase(ctx, svcCtx)}
}

func (l *RetireSeedLogic) RetireSeed(in *pb.SeedActionReq) (*pb.SeedResp, error) {
	return transitionSeed(l.base, in, store.SeedRetired)
}

func transitionSeed(b base, in *pb.SeedActionReq, to string) (*pb.SeedResp, error) {
	r, err := b.reviewer(in.GetUserId(), store.RolePolicyAdmin)
	if err != nil {
		return nil, err
	}
	seed, err := b.svcCtx.Store.TransitionSeed(b.ctx, in.GetSeedId(), r.UserID, to, b.svcCtx.Now())
	if err != nil {
		return nil, b.mapStoreError(err, "transition seed")
	}
	return &pb.SeedResp{Seed: seedView(seed)}, nil
}

func seedView(s *store.Seed) *pb.SeedView {
	return &pb.SeedView{
		SeedId: s.ID, IssueCode: s.IssueCode, Market: s.Market, Language: s.Language, Text: s.Text,
		Status: s.Status, NominatedBy: s.NominatedBy, ConfirmedBy: s.ConfirmedBy, SourceTaskId: s.SourceTaskID,
		UpdatedAtMs: s.UpdatedAtMs,
	}
}
