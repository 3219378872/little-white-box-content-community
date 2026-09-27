package media

import (
	"esx/app/gateway/internal/handler/mediaupload"
	"esx/app/gateway/internal/logic/media"
	"esx/app/gateway/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
)

func UploadVideoHandler(s *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer mediaupload.Cleanup(r)
		file, header, key, err := mediaupload.Bind(w, r, mediaupload.VideoLimit)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		defer file.Close()
		resp, err := media.NewUploadVideoLogic(r.Context(), s).UploadMultipart(file, header.Filename, key)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
