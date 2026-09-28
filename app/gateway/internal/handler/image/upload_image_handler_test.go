package image

import (
	"bytes"
	"context"
	"encoding/json"
	"esx/pkg/httptestx"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/cloudwego/kitex/client/callopt"
	"github.com/cloudwego/kitex/pkg/streaming"

	"esx/app/gateway/internal/svc"
	"esx/app/media/rpc/mediaservice"
	mediapb "esx/kitex_gen/media"
	"esx/pkg/errx"
	"esx/pkg/jwtx"
)

// fakeUploadStream / fakeMediaService 与 logic 测试相同结构（local copy 避免跨包导出）
type fakeUploadStream struct {
	streaming.Stream
	closeResp *mediapb.UploadImageResp
	closeErr  error
}

func (f *fakeUploadStream) Send(_ *mediapb.UploadImageReq) error { return nil }
func (f *fakeUploadStream) CloseAndRecv() (*mediapb.UploadImageResp, error) {
	return f.closeResp, f.closeErr
}

type fakeMediaService struct {
	mediaservice.MediaService
}

func (f *fakeMediaService) UploadImage(_ context.Context, _ ...callopt.Option) (mediaservice.MediaService_UploadImageClient, error) {
	return &fakeUploadStream{
		closeResp: &mediapb.UploadImageResp{
			Media: &mediapb.MediaInfo{Id: 1, Url: "u", ThumbnailUrl: "t"},
		},
	}, nil
}

func newSvcCtx() *svc.ServiceContext {
	return &svc.ServiceContext{MediaService: &fakeMediaService{}}
}

// makeMultipartRequest 构造一个 multipart/form-data 请求
func makeMultipartRequest(t *testing.T, formField, filename, contentType string, body []byte, authedUserID int64) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if formField != "" {
		hdr := make(textproto.MIMEHeader)
		hdr.Set("Content-Disposition", `form-data; name="`+formField+`"; filename="`+filename+`"`)
		hdr.Set("Content-Type", contentType)
		part, err := mw.CreatePart(hdr)
		if err != nil {
			t.Fatalf("CreatePart: %v", err)
		}
		if _, err := part.Write(body); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("Close mw: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload/image", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if authedUserID != 0 {
		ctx := jwtx.WithUserIdContext(req.Context(), authedUserID)
		req = req.WithContext(ctx)
	}
	return req
}

func extractBizCode(t *testing.T, body []byte) int {
	t.Helper()
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode public error: %v", err)
	}
	if payload.Message == "" {
		t.Fatal("public error message missing")
	}
	return payload.Code
}

func TestUploadImageHandler_NotMultipart_ReturnsParamError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload/image", strings.NewReader("not-multipart"))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()

	httptestx.Adapt(UploadImageHandler(newSvcCtx()))(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	if got := extractBizCode(t, body); got != errx.ParamError {
		t.Fatalf("expected code=%d ParamError, got %d (body=%s)", errx.ParamError, got, string(body))
	}
}

func TestUploadImageHandler_MissingFileField_ReturnsParamError(t *testing.T) {
	req := makeMultipartRequest(t, "other", "x.png", "image/png", []byte("hi"), 1)
	w := httptest.NewRecorder()

	httptestx.Adapt(UploadImageHandler(newSvcCtx()))(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	if got := extractBizCode(t, body); got != errx.ParamError {
		t.Fatalf("expected code=%d ParamError, got %d (body=%s)", errx.ParamError, got, string(body))
	}
}

func TestUploadImageHandler_IgnoredDeclaredContentType_StillUploads(t *testing.T) {
	req := makeMultipartRequest(t, "file", "x.bin", "application/octet-stream", []byte("hello"), 42)
	w := httptest.NewRecorder()

	httptestx.Adapt(UploadImageHandler(newSvcCtx()))(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("handler must not reject by Content-Type header, got %d body=%s", resp.StatusCode, string(body))
	}
}

func TestUploadImageHandler_Success_Returns200WithMediaInfo(t *testing.T) {
	req := makeMultipartRequest(t, "file", "x.png", "image/png", []byte("hello"), 42)
	w := httptest.NewRecorder()

	httptestx.Adapt(UploadImageHandler(newSvcCtx()))(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(body))
	}
	body, _ := io.ReadAll(resp.Body)
	var data struct {
		MediaId      int64  `json:"mediaId"`
		Url          string `json:"url"`
		ThumbnailUrl string `json:"thumbnailUrl"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatalf("unmarshal: %v, body=%s", err, string(body))
	}
	if data.MediaId != 1 || data.Url != "u" || data.ThumbnailUrl != "t" {
		t.Fatalf("unexpected data: %+v (body=%s)", data, string(body))
	}
}
