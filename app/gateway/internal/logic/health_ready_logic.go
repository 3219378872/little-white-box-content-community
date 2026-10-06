package logic

import (
	"context"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"

	"esx/pkg/logging"
)

type HealthReadyLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 就绪检查
func NewHealthReadyLogic(ctx context.Context, svcCtx *svc.ServiceContext) *HealthReadyLogic {
	return &HealthReadyLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// HealthReady 返回依赖状态（REL-053）。必需依赖故障 → unavailable；
// 可选发现能力故障 → degraded，但不使整个 Gateway 下线。
func (l *HealthReadyLogic) HealthReady(_ *types.HealthReq) (*types.HealthReadyResp, error) {
	if l.svcCtx == nil {
		return &types.HealthReadyResp{Status: "unavailable", Dependencies: map[string]string{}}, nil
	}
	status, dependencies := l.svcCtx.Readiness(l.ctx)
	byName := make(map[string]string, len(dependencies))
	for _, dependency := range dependencies {
		byName[dependency.Name] = dependency.Status
	}
	return &types.HealthReadyResp{Status: status, Dependencies: byName}, nil
}
