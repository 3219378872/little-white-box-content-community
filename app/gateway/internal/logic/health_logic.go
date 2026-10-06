package logic

import (
	"context"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"

	"esx/pkg/logging"
)

// HealthLogic 承载 Health 接口的业务逻辑；每个请求新建一个实例。
type HealthLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 健康检查
func NewHealthLogic(ctx context.Context, svcCtx *svc.ServiceContext) *HealthLogic {
	return &HealthLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Health 只表示网关进程存活；依赖就绪状态由 readiness 接口单独探测。
func (l *HealthLogic) Health(req *types.HealthReq) (resp *types.HealthResp, err error) {
	return &types.HealthResp{Status: "ok"}, nil
}
