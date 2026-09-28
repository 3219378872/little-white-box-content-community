package media

import (
	"context"
	"esx/app/gateway/internal/handler/mediaupload"
	"esx/app/gateway/internal/logic/media"
	"esx/app/gateway/internal/svc"
	"esx/pkg/httpx"
	"github.com/cloudwego/hertz/pkg/app"
)

func UploadVideoHandler(s *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		defer mediaupload.Cleanup(c)
		file, header, key, err := mediaupload.Bind(c, mediaupload.VideoLimit)
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
			return
		}
		defer file.Close()
		resp, err := media.NewUploadVideoLogic(ctx, s).UploadMultipart(file, header.Filename, key)
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
			return
		}
		httpx.OkJsonCtx(ctx, c, resp)
	}
}
