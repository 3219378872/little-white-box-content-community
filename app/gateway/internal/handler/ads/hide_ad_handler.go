package ads

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"

	"esx/app/gateway/internal/logic/ads"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/httpx"
)

// 付费广告：POST /api/v2/ads/{adId}/hide
func HideAdHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req types.HideAdReq
		if err := httpx.Parse(c, &req); err != nil {
			httpx.ErrorCtx(ctx, c, err)
			return
		}

		l := ads.NewHideAdLogic(ctx, svcCtx)
		resp, err := l.HideAd(&req)
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
		} else {
			httpx.OkJsonCtx(ctx, c, resp)
		}
	}
}
