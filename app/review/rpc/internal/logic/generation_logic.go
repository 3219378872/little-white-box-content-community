package logic

import (
	"context"
	"strconv"
	"time"

	"esx/app/review/rpc/internal/svc"
	pb "esx/kitex_gen/review"
)

// SeedIndexLag 覆盖种子确认后写入向量集合的延迟（worker 每 30 秒同步）：变化后这段时间内作出的
// 结论可能尚未看到新种子，仍按旧代次处理，由回扫兜底。
const SeedIndexLag = time.Minute

type GetRescanGenerationLogic struct{ base }

func NewGetRescanGenerationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRescanGenerationLogic {
	return &GetRescanGenerationLogic{newBase(ctx, svcCtx)}
}

// GetRescanGeneration 返回当前回扫代次（ADS-031）：政策版本与生效种子集合任一变化即产生新代次。
// 审核平台不了解业务对象全集，由业务方按代次枚举在投对象送回扫（DES-sponsored-ads「投后」）。
func (l *GetRescanGenerationLogic) GetRescanGeneration(_ *pb.GetRescanGenerationReq) (*pb.GetRescanGenerationResp, error) {
	version := l.svcCtx.Policy.Version
	activatedAt, found, err := l.svcCtx.Store.PolicyActivatedAt(l.ctx, version)
	if err != nil {
		return nil, l.mapStoreError(err, "load policy activation")
	}
	if !found {
		// 本进程的政策版本尚未被 worker 激活：生效路径仍是旧版本，此时回扫会按旧版本重审。
		return &pb.GetRescanGenerationResp{Ready: false}, nil
	}
	changes, changedAt, err := l.svcCtx.Store.SeedGeneration(l.ctx)
	if err != nil {
		return nil, l.mapStoreError(err, "load seed generation")
	}
	since := activatedAt
	if changes > 0 {
		since = max(since, changedAt+SeedIndexLag.Milliseconds())
	}
	return &pb.GetRescanGenerationResp{
		Ready: true, Generation: version + "+seeds@" + strconv.FormatInt(changes, 10), EffectiveSinceMs: since,
	}, nil
}
