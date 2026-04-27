//go:build integration

package logic

import (
	"bytes"
	"context"
	"errx"
	"esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// fakeUploadImageStream 模拟 pb.MediaService_UploadImageServer。
type fakeUploadImageStream struct {
	grpc.ServerStream
	reqs []*pb.UploadImageReq
	idx  int
	resp *pb.UploadImageResp
	ctx  context.Context
}

func (s *fakeUploadImageStream) Context() context.Context    { return s.ctx }
func (s *fakeUploadImageStream) SetHeader(metadata.MD) error { return nil }
func (s *fakeUploadImageStream) SendHeader(metadata.MD) error {
	return nil
}
func (s *fakeUploadImageStream) SetTrailer(metadata.MD) {}
func (s *fakeUploadImageStream) SendAndClose(r *pb.UploadImageResp) error {
	s.resp = r
	return nil
}
func (s *fakeUploadImageStream) Recv() (*pb.UploadImageReq, error) {
	if s.idx >= len(s.reqs) {
		return nil, io.EOF
	}
	req := s.reqs[s.idx]
	s.idx++
	return req, nil
}
func (s *fakeUploadImageStream) SendMsg(m interface{}) error { return nil }
func (s *fakeUploadImageStream) RecvMsg(m interface{}) error { return nil }

func encodeTestJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

func streamFromBytes(ctx context.Context, userId int64, filename string, data []byte, chunkSize int) *fakeUploadImageStream {
	reqs := []*pb.UploadImageReq{
		{Data: &pb.UploadImageReq_Meta{Meta: &pb.UploadMeta{
			UserId: userId, FileName: filename, Quality: 85, MaxWidth: 1000, MaxHeight: 1000,
		}}},
	}
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		reqs = append(reqs, &pb.UploadImageReq{Data: &pb.UploadImageReq_Chunk{Chunk: data[i:end]}})
	}
	return &fakeUploadImageStream{reqs: reqs, ctx: ctx}
}

func TestUploadImage_Success(t *testing.T) {
	ctx := context.Background()
	data := encodeTestJPEG(t, 1500, 1000)
	stream := streamFromBytes(ctx, 4001, "hello.jpg", data, 64*1024)

	l := NewUploadImageLogic(ctx, testSvcCtx)
	require.NoError(t, l.UploadImage(stream))

	require.NotNil(t, stream.resp)
	require.NotNil(t, stream.resp.Media)
	assert.Greater(t, stream.resp.Media.Id, int64(0))
	assert.LessOrEqual(t, stream.resp.Media.Width, int32(1000))
	assert.Contains(t, stream.resp.Media.Url, "original/")
	assert.Contains(t, stream.resp.Media.ThumbnailUrl, "thumb/")
}

func TestUploadImage_MetaMissing(t *testing.T) {
	ctx := context.Background()
	stream := &fakeUploadImageStream{
		ctx: ctx,
		reqs: []*pb.UploadImageReq{
			{Data: &pb.UploadImageReq_Chunk{Chunk: []byte("hello")}},
		},
	}
	l := NewUploadImageLogic(ctx, testSvcCtx)
	err := l.UploadImage(stream)
	assertBizError(t, err, errx.MediaMetaMissing)
}

func TestUploadImage_SizeExceeded(t *testing.T) {
	ctx := context.Background()
	data := bytes.Repeat([]byte{0xFF, 0xD8, 0xFF, 0xE0}, 11*256*1024)
	stream := streamFromBytes(ctx, 4002, "big.jpg", data, 1024*1024)

	l := NewUploadImageLogic(ctx, testSvcCtx)
	err := l.UploadImage(stream)
	assertBizError(t, err, errx.FileTooLarge)
}

func TestUploadImage_TypeNotAllowed(t *testing.T) {
	ctx := context.Background()
	data := append([]byte{0x25, 0x50, 0x44, 0x46, 0x2D, 0x31, 0x2E, 0x34}, bytes.Repeat([]byte{0x00}, 1024)...)
	stream := streamFromBytes(ctx, 4003, "doc.pdf", data, 512)

	l := NewUploadImageLogic(ctx, testSvcCtx)
	err := l.UploadImage(stream)
	assertBizError(t, err, errx.FileTypeNotAllowed)
}
