// Package mediaupload binds bounded multipart uploads for all media routes.
package mediaupload

import (
	"errors"
	"esx/pkg/errx"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

const ImageLimit int64 = 10 << 20
const AudioLimit int64 = 10 << 20
const VideoLimit int64 = 100 << 20
const formOverhead int64 = 1 << 20

// Bind spools large files to disk. Callers must defer Cleanup even on errors.
func Bind(c *app.RequestContext, limit int64) (multipart.File, *multipart.FileHeader, string, error) {
	// Reject non-multipart bodies before reading anything.
	contentType, params, err := mime.ParseMediaType(string(c.Request.Header.ContentType()))
	if err != nil || contentType != "multipart/form-data" || params["boundary"] == "" {
		return nil, nil, "", errx.NewWithCode(errx.ParamError)
	}
	reader := c.Request.BodyStream()
	if reader == nil {
		body, e := c.Request.BodyE()
		if e != nil {
			return nil, nil, "", errx.NewWithCode(errx.ParamError)
		}
		reader = strings.NewReader(string(body))
	}
	// Cap the raw body at limit plus form overhead; parts beyond 1MiB spill to temp files.
	form, err := multipart.NewReader(&limitedReader{reader: reader, remaining: limit + formOverhead}, params["boundary"]).ReadForm(1 << 20)
	// Register the form before checking err so Cleanup also removes partially written temp files.
	if form != nil {
		c.Set("uploadForm", form)
	}
	if err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			return nil, nil, "", errx.NewWithCode(errx.FileTooLarge)
		}
		return nil, nil, "", errx.NewWithCode(errx.ParamError)
	}
	// Exactly one "file" part and at most one idempotency key are accepted.
	if len(form.File) != 1 || len(form.File["file"]) != 1 || len(form.Value["idempotencyKey"]) > 1 {
		return nil, nil, "", errx.NewWithCode(errx.ParamError)
	}
	header := form.File["file"][0]
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
	key := ""
	if len(form.Value["idempotencyKey"]) == 1 {
		key = form.Value["idempotencyKey"][0]
	}
	return file, header, key, nil
}

// Cleanup removes the temp files of the bound form; safe to call when Bind failed or never ran.
func Cleanup(c *app.RequestContext) {
	if raw, ok := c.Get("uploadForm"); ok {
		if form := raw.(*multipart.Form); form != nil {
			_ = form.RemoveAll()
		}
		c.Set("uploadForm", (*multipart.Form)(nil))
	}
}

// limitedReader caps the body like http.MaxBytesReader, which Hertz streams cannot use.
type limitedReader struct {
	reader    io.Reader
	remaining int64
}

// Read returns MaxBytesError only when data actually exceeds the cap, so a body
// that ends exactly at the limit still reaches EOF normally.
func (r *limitedReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		var extra [1]byte
		n, e := r.reader.Read(extra[:])
		if n > 0 {
			return 0, &http.MaxBytesError{Limit: 0}
		}
		return 0, e
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, e := r.reader.Read(p)
	r.remaining -= int64(n)
	return n, e
}
