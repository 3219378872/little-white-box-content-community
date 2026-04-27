package image

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/textproto"
	"testing"

	"errx"
	"esx/app/media/rpc/mediaservice"
	mediapb "esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"gateway/internal/svc"
	"jwtx"

	"google.golang.org/grpc"
)

// fakeUploadStream 实现 grpc.ClientStreamingClient[mediapb.UploadImageReq, mediapb.UploadImageResp]
type fakeUploadStream struct {
	grpc.ClientStream // 嵌入接口零值；测试中不使用 stream 元方法
	sentMetas         []*mediapb.UploadMeta
	sentChunks        [][]byte
	sendErr           error
	sendErrAfter      int // 第几次 Send 后开始返回 sendErr（0 表示从首包就错）
	sendCount         int
	closeResp         *mediapb.UploadImageResp
	closeErr          error
}

func (f *fakeUploadStream) Send(req *mediapb.UploadImageReq) error {
	f.sendCount++
	if f.sendErr != nil && f.sendCount > f.sendErrAfter {
		return f.sendErr
	}
	if m := req.GetMeta(); m != nil {
		f.sentMetas = append(f.sentMetas, m)
	} else if c := req.GetChunk(); c != nil {
		chunk := make([]byte, len(c))
		copy(chunk, c)
		f.sentChunks = append(f.sentChunks, chunk)
	}
	return nil
}

func (f *fakeUploadStream) CloseAndRecv() (*mediapb.UploadImageResp, error) {
	return f.closeResp, f.closeErr
}

// fakeMediaService 仅覆盖 UploadImage
type fakeMediaService struct {
	mediaservice.MediaService
	uploadFn func(ctx context.Context) (mediapb.MediaService_UploadImageClient, error)
}

func (f *fakeMediaService) UploadImage(ctx context.Context, _ ...grpc.CallOption) (mediapb.MediaService_UploadImageClient, error) {
	return f.uploadFn(ctx)
}

// makeFile 构造一个 in-memory multipart 文件用于测试
func makeFile(t *testing.T, name string, body []byte) (multipart.File, *multipart.FileHeader) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="`+name+`"`)
	hdr.Set("Content-Type", "image/png")
	part, err := mw.CreatePart(hdr)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatalf("Write part: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("Close mw: %v", err)
	}
	mr := multipart.NewReader(&buf, mw.Boundary())
	form, err := mr.ReadForm(int64(len(body)) + 1024)
	if err != nil {
		t.Fatalf("ReadForm: %v", err)
	}
	headers := form.File["file"]
	if len(headers) == 0 {
		t.Fatal("no file in form")
	}
	file, err := headers[0].Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return file, headers[0]
}

func buildUploadLogic(userID int64, ms mediaservice.MediaService) *UploadImageLogic {
	svcCtx := &svc.ServiceContext{MediaService: ms}
	ctx := context.Background()
	if userID != 0 {
		ctx = jwtx.WithUserIdContext(ctx, userID)
	}
	return NewUploadImageLogic(ctx, svcCtx)
}

func TestUploadImageMultipart_Unauthenticated_ReturnsLoginRequired(t *testing.T) {
	file, header := makeFile(t, "a.png", []byte("hello"))
	defer file.Close()
	l := buildUploadLogic(0, &fakeMediaService{})
	_, err := l.UploadImageMultipart(file, header)
	if !errx.Is(err, errx.LoginRequired) {
		t.Fatalf("expected LoginRequired, got: %v", err)
	}
}

func TestUploadImageMultipart_Success_SendsMetaAndChunks(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 3*(1<<20)+123) // 3MB + 123 → 4 个 chunk
	file, header := makeFile(t, "big.png", body)
	defer file.Close()

	stream := &fakeUploadStream{
		closeResp: &mediapb.UploadImageResp{
			Media: &mediapb.MediaInfo{Id: 99, Url: "https://cdn/x.png", ThumbnailUrl: "https://cdn/x_t.png"},
		},
	}
	ms := &fakeMediaService{
		uploadFn: func(_ context.Context) (mediapb.MediaService_UploadImageClient, error) {
			return stream, nil
		},
	}
	l := buildUploadLogic(42, ms)

	resp, err := l.UploadImageMultipart(file, header)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.MediaId != 99 || resp.Url != "https://cdn/x.png" || resp.ThumbnailUrl != "https://cdn/x_t.png" {
		t.Fatalf("unexpected resp: %+v", resp)
	}
	if len(stream.sentMetas) != 1 {
		t.Fatalf("expected 1 meta, got %d", len(stream.sentMetas))
	}
	if stream.sentMetas[0].UserId != 42 {
		t.Fatalf("expected UserId=42 in meta, got %d", stream.sentMetas[0].UserId)
	}
	if stream.sentMetas[0].FileName != "big.png" {
		t.Fatalf("expected FileName=big.png, got %s", stream.sentMetas[0].FileName)
	}
	totalSent := 0
	for _, c := range stream.sentChunks {
		totalSent += len(c)
	}
	if totalSent != len(body) {
		t.Fatalf("expected total bytes sent=%d, got %d", len(body), totalSent)
	}
	if len(stream.sentChunks) != 4 {
		t.Fatalf("expected 4 chunks for 3MB+123 body, got %d", len(stream.sentChunks))
	}
}

func TestUploadImageMultipart_StreamSetupError_WrapsError(t *testing.T) {
	file, header := makeFile(t, "a.png", []byte("hi"))
	defer file.Close()
	ms := &fakeMediaService{
		uploadFn: func(_ context.Context) (mediapb.MediaService_UploadImageClient, error) {
			return nil, errors.New("dial failed")
		},
	}
	l := buildUploadLogic(7, ms)
	_, err := l.UploadImageMultipart(file, header)
	if !errx.Is(err, errx.SystemError) {
		t.Fatalf("expected SystemError, got: %v", err)
	}
}

func TestUploadImageMultipart_SendMetaError_WrapsError(t *testing.T) {
	file, header := makeFile(t, "a.png", []byte("hi"))
	defer file.Close()
	stream := &fakeUploadStream{sendErr: errors.New("broken pipe")}
	ms := &fakeMediaService{
		uploadFn: func(_ context.Context) (mediapb.MediaService_UploadImageClient, error) {
			return stream, nil
		},
	}
	l := buildUploadLogic(7, ms)
	_, err := l.UploadImageMultipart(file, header)
	if !errx.Is(err, errx.UploadFailed) {
		t.Fatalf("expected UploadFailed, got: %v", err)
	}
}

func TestUploadImageMultipart_NilMediaInResponse_ReturnsUploadFailed(t *testing.T) {
	file, header := makeFile(t, "a.png", []byte("hi"))
	defer file.Close()
	stream := &fakeUploadStream{closeResp: &mediapb.UploadImageResp{Media: nil}}
	ms := &fakeMediaService{
		uploadFn: func(_ context.Context) (mediapb.MediaService_UploadImageClient, error) {
			return stream, nil
		},
	}
	l := buildUploadLogic(7, ms)
	_, err := l.UploadImageMultipart(file, header)
	if !errx.Is(err, errx.UploadFailed) {
		t.Fatalf("expected UploadFailed, got: %v", err)
	}
}

// 防止 io 包 import 被未引用警告
var _ = io.EOF
