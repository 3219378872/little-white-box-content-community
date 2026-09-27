// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package image

import (
	"esx/app/gateway/internal/handler/mediaupload"
	"net/http"

	"esx/app/gateway/internal/logic/image"
	"esx/app/gateway/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// UploadImageHandler 上传图片（multipart/form-data，字段名 file）
func UploadImageHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer mediaupload.Cleanup(r)
		file, header, key, err := mediaupload.Bind(w, r, mediaupload.ImageLimit)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		defer file.Close()

		// CORE-023：按文件内容识别类型，不在 Handler 用 Content-Type 头拦截。
		l := image.NewUploadImageLogic(r.Context(), svcCtx)
		resp, err := l.UploadImageMultipart(file, header, key)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
