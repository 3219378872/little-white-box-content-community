package handler

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"

	"esx/app/gateway/internal/logic"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"

	"esx/pkg/httpx"
)

// 就绪检查
func HealthReadyHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		l := logic.NewHealthReadyLogic(ctx, svcCtx)
		resp, err := l.HealthReady(&types.HealthReq{})
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
		} else {
			httpx.OkJsonCtx(ctx, c, resp)
		}
	}
}
