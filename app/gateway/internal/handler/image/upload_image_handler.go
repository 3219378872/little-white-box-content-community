package image

import (
	"context"
	"esx/app/gateway/internal/handler/mediaupload"
	"github.com/cloudwego/hertz/pkg/app"

	"esx/app/gateway/internal/logic/image"
	"esx/app/gateway/internal/svc"

	"esx/pkg/httpx"
)

// UploadImageHandler 上传图片（multipart/form-data，字段名 file）
func UploadImageHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		defer mediaupload.Cleanup(c)
		file, header, key, err := mediaupload.Bind(c, mediaupload.ImageLimit)
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
			return
		}
		defer file.Close()

		// CORE-023：按文件内容识别类型，不在 Handler 用 Content-Type 头拦截。
		l := image.NewUploadImageLogic(ctx, svcCtx)
		resp, err := l.UploadImageMultipart(file, header, key)
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
			return
		}
		httpx.OkJsonCtx(ctx, c, resp)
	}
}
