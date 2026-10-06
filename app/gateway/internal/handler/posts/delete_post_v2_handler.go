package posts

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"

	"esx/app/gateway/internal/logic/posts"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/httpx"
)

// 删除帖子（v2，强制 expectedRevision）
func DeletePostV2Handler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req types.DeletePostV2Req
		if err := httpx.Parse(c, &req); err != nil {
			httpx.ErrorCtx(ctx, c, err)
			return
		}

		l := posts.NewDeletePostV2Logic(ctx, svcCtx)
		resp, err := l.DeletePostV2(&req)
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
		} else {
			httpx.OkJsonCtx(ctx, c, resp)
		}
	}
}
