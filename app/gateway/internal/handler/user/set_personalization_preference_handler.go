package user

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"

	"esx/app/gateway/internal/logic/user"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"

	"esx/pkg/httpx"
)

// 设置个性化偏好
func SetPersonalizationPreferenceHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req types.SetPersonalizationPreferenceReq
		if err := httpx.Parse(c, &req); err != nil {
			httpx.ErrorCtx(ctx, c, err)
			return
		}

		l := user.NewSetPersonalizationPreferenceLogic(ctx, svcCtx)
		resp, err := l.SetPersonalizationPreference(&req)
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
		} else {
			httpx.OkJsonCtx(ctx, c, resp)
		}
	}
}
