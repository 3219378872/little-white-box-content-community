package mediaupload

import (
	"bytes"
	"context"
	"esx/app/gateway/internal/svc"
	"esx/app/media/rpc/mediaservice"
	pb "esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"esx/pkg/errx"
	"esx/pkg/jwtx"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type videoStream struct {
	grpc.ClientStreamingClient[pb.UploadVideoReq, pb.UploadVideoResp]
	meta *pb.UploadMeta
	size int
	err  error
}

func (s *videoStream) Send(r *pb.UploadVideoReq) error {
	if r.GetMeta() != nil {
		s.meta = r.GetMeta()
	}
	s.size += len(r.GetChunk())
	return nil
}
func (s *videoStream) CloseAndRecv() (*pb.UploadVideoResp, error) {
	return &pb.UploadVideoResp{Media: &pb.MediaInfo{Id: 42, Url: "https://media.test/v", FileType: "video", FileSize: int64(s.size)}}, s.err
}

type audioStream struct {
	grpc.ClientStreamingClient[pb.UploadAudioReq, pb.UploadAudioResp]
	meta *pb.UploadMeta
	size int
	err  error
}

func (s *audioStream) Send(r *pb.UploadAudioReq) error {
	if r.GetMeta() != nil {
		s.meta = r.GetMeta()
	}
	s.size += len(r.GetChunk())
	return nil
}
func (s *audioStream) CloseAndRecv() (*pb.UploadAudioResp, error) {
	return &pb.UploadAudioResp{Media: &pb.MediaInfo{Id: 43, Url: "https://media.test/a", FileType: "audio", FileSize: int64(s.size)}}, s.err
}

type mediaService struct {
	mediaservice.MediaService
	v *videoStream
	a *audioStream
}

func (s *mediaService) UploadVideo(context.Context, ...grpc.CallOption) (pb.MediaService_UploadVideoClient, error) {
	return s.v, nil
}
func (s *mediaService) UploadAudio(context.Context, ...grpc.CallOption) (pb.MediaService_UploadAudioClient, error) {
	return s.a, nil
}
func TestUploadStreamingMetadataChunksAndFailures(t *testing.T) {
	for _, audio := range []bool{false, true} {
		service := &mediaService{v: &videoStream{}, a: &audioStream{}}
		s := &svc.ServiceContext{MediaService: service}
		ctx := jwtx.WithUserIdContext(context.Background(), 7)
		data := make([]byte, (1<<20)+13)
		resp, err := Upload(ctx, s, bytes.NewReader(data), "file", "stable", audio)
		require.NoError(t, err)
		require.Equal(t, int64(len(data)), resp.FileSize)
		meta := service.v.meta
		if audio {
			meta = service.a.meta
		}
		require.Equal(t, int64(7), meta.UserId)
		require.Equal(t, "stable", meta.IdempotencyKey)
		_, err = Upload(ctx, s, bytes.NewReader(data), "file", "", audio)
		require.True(t, errx.Is(err, errx.ParamError))
		_, err = Upload(context.Background(), s, bytes.NewReader(data), "file", "k", audio)
		require.True(t, errx.Is(err, errx.LoginRequired))
		service.v.err = (&errx.BizError{Code: errx.FileTypeNotAllowed, Message: "type"}).GRPCStatus().Err()
		service.a.err = service.v.err
		_, err = Upload(ctx, s, bytes.NewReader(data), "file", "k", audio)
		require.True(t, errx.Is(err, errx.FileTypeNotAllowed))
	}
}
