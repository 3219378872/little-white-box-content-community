package user

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"

	"esx/app/gateway/internal/logic/user"
	"esx/app/gateway/internal/svc"

	"esx/pkg/httpx"
)

// 获取个性化偏好
func GetPersonalizationPreferenceHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		l := user.NewGetPersonalizationPreferenceLogic(ctx, svcCtx)
		resp, err := l.GetPersonalizationPreference()
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
		} else {
			httpx.OkJsonCtx(ctx, c, resp)
		}
	}
}
