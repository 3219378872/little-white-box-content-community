// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package image

import (
	"errx"
	"net/http"

	"gateway/internal/logic/image"
	"gateway/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

const maxUploadSize = 10 << 20 // 10 MB

var allowedImageTypes = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

// UploadImageHandler 上传图片（multipart/form-data，字段名 file）
func UploadImageHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			httpx.ErrorCtx(r.Context(), w, errx.NewWithCode(errx.FileTooLarge))
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, errx.NewWithCode(errx.ParamError))
			return
		}
		defer file.Close()

		ct := header.Header.Get("Content-Type")
		if _, ok := allowedImageTypes[ct]; !ok {
			httpx.ErrorCtx(r.Context(), w, errx.NewWithCode(errx.FileTypeNotAllowed))
			return
		}

		l := image.NewUploadImageLogic(r.Context(), svcCtx)
		resp, err := l.UploadImageMultipart(file, header)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
