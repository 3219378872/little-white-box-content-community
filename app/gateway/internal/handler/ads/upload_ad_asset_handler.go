package ads

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"

	"esx/app/gateway/internal/handler/mediaupload"
	"esx/app/gateway/internal/logic/ads"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/httpx"
)

// 付费广告：上传素材或资质证件到私有存储（ADS-015）
func UploadAdAssetHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		defer mediaupload.Cleanup(c)
		req := types.UploadAdAssetReq{Kind: c.Param("kind")}
		file, header, key, err := mediaupload.Bind(c, ads.MaxAssetBytes)
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
			return
		}
		defer file.Close()
		resp, err := ads.NewUploadAdAssetLogic(ctx, svcCtx).UploadAdAsset(&req, file, header.Filename, key)
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
		} else {
			httpx.OkJsonCtx(ctx, c, resp)
		}
	}
}
