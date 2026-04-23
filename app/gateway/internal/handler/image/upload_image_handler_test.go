package image

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"errx"
	mediapb "esx/app/media/pb/xiaobaihe/media/pb"
	"esx/app/media/mediaservice"
	"gateway/internal/svc"
	"jwtx"

	"google.golang.org/grpc"
)

// httpx.ErrorCtx 默认把 BizError.Error() 写成 "code: N, message: ..." 文本
var bizErrRe = regexp.MustCompile(`code:\s*(\d+),\s*message:`)

// fakeUploadStream / fakeMediaService 与 logic 测试相同结构（local copy 避免跨包导出）
type fakeUploadStream struct {
	grpc.ClientStream
	closeResp *mediapb.UploadImageResp
	closeErr  error
}

func (f *fakeUploadStream) Send(_ *mediapb.UploadImageReq) error      { return nil }
func (f *fakeUploadStream) CloseAndRecv() (*mediapb.UploadImageResp, error) {
	return f.closeResp, f.closeErr
}

type fakeMediaService struct {
	mediaservice.MediaService
}

func (f *fakeMediaService) UploadImage(_ context.Context, _ ...grpc.CallOption) (mediapb.MediaService_UploadImageClient, error) {
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
	m := bizErrRe.FindSubmatch(body)
	if len(m) < 2 {
		t.Fatalf("biz error pattern not found, body=%s", string(body))
	}
	code, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatalf("parse code: %v", err)
	}
	return code
}

func TestUploadImageHandler_NotMultipart_ReturnsFileTooLarge(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload/image", strings.NewReader("not-multipart"))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()

	UploadImageHandler(newSvcCtx())(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	if got := extractBizCode(t, body); got != errx.FileTooLarge {
		t.Fatalf("expected code=%d FileTooLarge, got %d (body=%s)", errx.FileTooLarge, got, string(body))
	}
}

func TestUploadImageHandler_MissingFileField_ReturnsParamError(t *testing.T) {
	req := makeMultipartRequest(t, "other", "x.png", "image/png", []byte("hi"), 1)
	w := httptest.NewRecorder()

	UploadImageHandler(newSvcCtx())(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	if got := extractBizCode(t, body); got != errx.ParamError {
		t.Fatalf("expected code=%d ParamError, got %d (body=%s)", errx.ParamError, got, string(body))
	}
}

func TestUploadImageHandler_DisallowedContentType_ReturnsFileTypeNotAllowed(t *testing.T) {
	req := makeMultipartRequest(t, "file", "x.gif", "image/gif", []byte("hi"), 1)
	w := httptest.NewRecorder()

	UploadImageHandler(newSvcCtx())(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	if got := extractBizCode(t, body); got != errx.FileTypeNotAllowed {
		t.Fatalf("expected code=%d FileTypeNotAllowed, got %d (body=%s)", errx.FileTypeNotAllowed, got, string(body))
	}
}

func TestUploadImageHandler_Success_Returns200WithMediaInfo(t *testing.T) {
	req := makeMultipartRequest(t, "file", "x.png", "image/png", []byte("hello"), 42)
	w := httptest.NewRecorder()

	UploadImageHandler(newSvcCtx())(w, req)

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
