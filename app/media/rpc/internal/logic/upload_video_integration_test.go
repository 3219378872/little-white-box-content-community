//go:build integration

package logic

import (
	"bytes"
	"context"
	"errx"
	"esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type fakeUploadVideoStream struct {
	grpc.ServerStream
	reqs []*pb.UploadVideoReq
	idx  int
	resp *pb.UploadVideoResp
	ctx  context.Context
}

func (s *fakeUploadVideoStream) Context() context.Context    { return s.ctx }
func (s *fakeUploadVideoStream) SetHeader(metadata.MD) error { return nil }
func (s *fakeUploadVideoStream) SendHeader(metadata.MD) error {
	return nil
}
func (s *fakeUploadVideoStream) SetTrailer(metadata.MD) {}
func (s *fakeUploadVideoStream) SendAndClose(r *pb.UploadVideoResp) error {
	s.resp = r
	return nil
}
func (s *fakeUploadVideoStream) Recv() (*pb.UploadVideoReq, error) {
	if s.idx >= len(s.reqs) {
		return nil, io.EOF
	}
	req := s.reqs[s.idx]
	s.idx++
	return req, nil
}
func (s *fakeUploadVideoStream) SendMsg(m interface{}) error { return nil }
func (s *fakeUploadVideoStream) RecvMsg(m interface{}) error { return nil }

// fakeMP4 构造一个最小的 MP4 魔数 + 填充数据。
func fakeMP4(paddingBytes int) []byte {
	head := []byte{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70, 0x6D, 0x70, 0x34, 0x32,
		0x00, 0x00, 0x00, 0x00, 0x6D, 0x70, 0x34, 0x31, 0x6D, 0x70, 0x34, 0x32}
	return append(head, bytes.Repeat([]byte{0x00}, paddingBytes)...)
}

func videoStreamFromBytes(ctx context.Context, userId int64, filename string, data []byte, chunkSize int) *fakeUploadVideoStream {
	reqs := []*pb.UploadVideoReq{
		{Data: &pb.UploadVideoReq_Meta{Meta: &pb.UploadMeta{
			UserId: userId, FileName: filename,
		}}},
	}
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		reqs = append(reqs, &pb.UploadVideoReq{Data: &pb.UploadVideoReq_Chunk{Chunk: data[i:end]}})
	}
	return &fakeUploadVideoStream{reqs: reqs, ctx: ctx}
}

func TestUploadVideo_Success(t *testing.T) {
	ctx := context.Background()
	data := fakeMP4(2048)
	stream := videoStreamFromBytes(ctx, 5001, "demo.mp4", data, 1024)

	l := NewUploadVideoLogic(ctx, testSvcCtx)
	require.NoError(t, l.UploadVideo(stream))

	require.NotNil(t, stream.resp)
	require.NotNil(t, stream.resp.Media)
	assert.Greater(t, stream.resp.Media.Id, int64(0))
	assert.Equal(t, "video", stream.resp.Media.FileType)
	assert.Contains(t, stream.resp.Media.Url, "original/")
}

func TestUploadVideo_TypeNotAllowed(t *testing.T) {
	ctx := context.Background()
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	data = append(data, bytes.Repeat([]byte{0x00}, 256)...)
	stream := videoStreamFromBytes(ctx, 5002, "fake.mp4", data, 128)

	l := NewUploadVideoLogic(ctx, testSvcCtx)
	err := l.UploadVideo(stream)
	assertBizError(t, err, errx.FileTypeNotAllowed)
}

func TestUploadVideo_SizeExceeded(t *testing.T) {
	ctx := context.Background()
	data := fakeMP4(101 * 1024 * 1024)
	stream := videoStreamFromBytes(ctx, 5003, "big.mp4", data, 1024*1024)

	l := NewUploadVideoLogic(ctx, testSvcCtx)
	err := l.UploadVideo(stream)
	assertBizError(t, err, errx.FileTooLarge)
}
