// Package mediaupload binds bounded multipart uploads for all media routes.
package mediaupload

import (
	"errors"
	"esx/pkg/errx"
	"mime/multipart"
	"net/http"
)

const ImageLimit int64 = 10 << 20
const AudioLimit int64 = 10 << 20
const VideoLimit int64 = 100 << 20
const formOverhead int64 = 1 << 20

// Bind spools large files to disk. Callers must defer Cleanup even on errors.
func Bind(w http.ResponseWriter, r *http.Request, limit int64) (multipart.File, *multipart.FileHeader, string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, limit+formOverhead)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			return nil, nil, "", errx.NewWithCode(errx.FileTooLarge)
		}
		return nil, nil, "", errx.NewWithCode(errx.ParamError)
	}
	if r.MultipartForm == nil || len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 || len(r.MultipartForm.Value["idempotencyKey"]) > 1 {
		return nil, nil, "", errx.NewWithCode(errx.ParamError)
	}
	header := r.MultipartForm.File["file"][0]
	if header.Size > limit {
		return nil, nil, "", errx.NewWithCode(errx.FileTooLarge)
	}
	if header.Size <= 0 {
		return nil, nil, "", errx.NewWithCode(errx.ParamError)
	}
	file, err := header.Open()
	if err != nil {
		return nil, nil, "", errx.NewWithCode(errx.UploadFailed)
	}
	return file, header, r.FormValue("idempotencyKey"), nil
}

func Cleanup(r *http.Request) {
	if r.MultipartForm != nil {
		_ = r.MultipartForm.RemoveAll()
	}
}
