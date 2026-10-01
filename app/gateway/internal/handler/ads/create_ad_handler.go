package ads

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"

	"esx/app/gateway/internal/logic/ads"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/httpx"
)

// 付费广告：POST /api/v2/ads
func CreateAdHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req types.CreateAdReq
		if err := httpx.Parse(c, &req); err != nil {
			httpx.ErrorCtx(ctx, c, err)
			return
		}

		l := ads.NewCreateAdLogic(ctx, svcCtx)
		resp, err := l.CreateAd(&req)
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
		} else {
			httpx.OkJsonCtx(ctx, c, resp)
		}
	}
}
