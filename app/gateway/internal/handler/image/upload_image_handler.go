// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package image

import (
	"errors"
	"esx/pkg/errx"
	"net/http"

	"esx/app/gateway/internal/logic/image"
	"esx/app/gateway/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

const maxUploadSize = 10 << 20 // 10 MB

// UploadImageHandler 上传图片（multipart/form-data，字段名 file）
func UploadImageHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			var maxBytes *http.MaxBytesError
			if errors.As(err, &maxBytes) {
				httpx.ErrorCtx(r.Context(), w, errx.NewWithCode(errx.FileTooLarge))
				return
			}
			httpx.ErrorCtx(r.Context(), w, errx.NewWithCode(errx.ParamError))
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, errx.NewWithCode(errx.ParamError))
			return
		}
		defer file.Close()

		// CORE-023：按文件内容识别类型，不在 Handler 用 Content-Type 头拦截。
		l := image.NewUploadImageLogic(r.Context(), svcCtx)
		resp, err := l.UploadImageMultipart(file, header, r.FormValue("idempotencyKey"))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
