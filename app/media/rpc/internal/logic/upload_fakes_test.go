package logic

import (
	"bytes"
	"context"
	"esx/app/media/rpc/internal/config"
	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/storage"
	"esx/app/media/rpc/internal/svc"
	pb "esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"esx/pkg/errx"
	"esx/pkg/util"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var unitSnowflakeOnce sync.Once

// unitInitSnowflake 为走 util.NextID 的成功路径用例初始化 ID 生成器。
func unitInitSnowflake(t *testing.T) {
	t.Helper()
	unitSnowflakeOnce.Do(func() {
		if err := util.InitSnowflake(1, 1); err != nil {
			t.Fatalf("init snowflake: %v", err)
		}
	})
}

// unitObjectStorage 是 storage.ObjectStorage 的内存替身，记录调用并支持注入失败。
type unitObjectStorage struct {
	putCalls   []unitPutCall
	deleteKeys []string

	putErrOn    int // 第 N 次 Put 返回错误（1 起；0 表示不注入）
	publicBase  string
	failDeletes bool
}

type unitPutCall struct {
	objectKey   string
	size        int64
	contentType string
}

func (s *unitObjectStorage) Put(ctx context.Context, objectKey string, reader io.Reader, size int64, contentType string) error {
	if s.putErrOn > 0 && len(s.putCalls)+1 == s.putErrOn {
		return fmt.Errorf("unit: injected put failure for %s", objectKey)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	s.putCalls = append(s.putCalls, unitPutCall{
		objectKey:   objectKey,
		size:        size,
		contentType: contentType,
	})
	if len(data) == 0 {
		return fmt.Errorf("unit: empty object body for %s", objectKey)
	}
	return nil
}

func (s *unitObjectStorage) Delete(ctx context.Context, objectKey string) error {
	if s.failDeletes {
		return fmt.Errorf("unit: injected delete failure for %s", objectKey)
	}
	s.deleteKeys = append(s.deleteKeys, objectKey)
	return nil
}

func (s *unitObjectStorage) BuildPublicURL(objectKey string) string {
	base := s.publicBase
	if base == "" {
		base = "http://storage.test/bucket"
	}
	return base + "/" + objectKey
}

// unitImageStream 模拟 pb.MediaService_UploadImageServer。
type unitImageStream struct {
	grpc.ClientStreamingServer[pb.UploadImageReq, pb.UploadImageResp]
	reqs []*pb.UploadImageReq
	idx  int
	resp *pb.UploadImageResp
	ctx  context.Context
}

func (s *unitImageStream) Context() context.Context    { return s.ctx }
func (s *unitImageStream) SetHeader(metadata.MD) error { return nil }
func (s *unitImageStream) SendHeader(metadata.MD) error {
	return nil
}
func (s *unitImageStream) SetTrailer(metadata.MD) {}
func (s *unitImageStream) SendAndClose(r *pb.UploadImageResp) error {
	s.resp = r
	return nil
}
func (s *unitImageStream) Recv() (*pb.UploadImageReq, error) {
	if s.idx >= len(s.reqs) {
		return nil, io.EOF
	}
	req := s.reqs[s.idx]
	s.idx++
	return req, nil
}

// unitVideoStream 模拟 pb.MediaService_UploadVideoServer。
type unitVideoStream struct {
	grpc.ClientStreamingServer[pb.UploadVideoReq, pb.UploadVideoResp]
	reqs []*pb.UploadVideoReq
	idx  int
	resp *pb.UploadVideoResp
	ctx  context.Context
}

func (s *unitVideoStream) Context() context.Context    { return s.ctx }
func (s *unitVideoStream) SetHeader(metadata.MD) error { return nil }
func (s *unitVideoStream) SendHeader(metadata.MD) error {
	return nil
}
func (s *unitVideoStream) SetTrailer(metadata.MD) {}
func (s *unitVideoStream) SendAndClose(r *pb.UploadVideoResp) error {
	s.resp = r
	return nil
}
func (s *unitVideoStream) Recv() (*pb.UploadVideoReq, error) {
	if s.idx >= len(s.reqs) {
		return nil, io.EOF
	}
	req := s.reqs[s.idx]
	s.idx++
	return req, nil
}

func unitMetaReq(userId int64, filename, idemKey string) *pb.UploadMeta {
	return &pb.UploadMeta{
		UserId:         userId,
		FileName:       filename,
		IdempotencyKey: idemKey,
		Quality:        85,
		MaxWidth:       1000,
		MaxHeight:      1000,
	}
}

func unitImageStreamFromBytes(ctx context.Context, userId int64, filename, idemKey string, data []byte, chunkSize int) *unitImageStream {
	reqs := []*pb.UploadImageReq{
		{Data: &pb.UploadImageReq_Meta{Meta: unitMetaReq(userId, filename, idemKey)}},
	}
	for i := 0; i < len(data); i += chunkSize {
		end := min(i+chunkSize, len(data))
		reqs = append(reqs, &pb.UploadImageReq{Data: &pb.UploadImageReq_Chunk{Chunk: data[i:end]}})
	}
	return &unitImageStream{reqs: reqs, ctx: ctx}
}

func unitVideoStreamFromBytes(ctx context.Context, userId int64, filename, idemKey string, data []byte, chunkSize int) *unitVideoStream {
	reqs := []*pb.UploadVideoReq{
		{Data: &pb.UploadVideoReq_Meta{Meta: unitMetaReq(userId, filename, idemKey)}},
	}
	for i := 0; i < len(data); i += chunkSize {
		end := min(i+chunkSize, len(data))
		reqs = append(reqs, &pb.UploadVideoReq{Data: &pb.UploadVideoReq_Chunk{Chunk: data[i:end]}})
	}
	return &unitVideoStream{reqs: reqs, ctx: ctx}
}

// unitTestJPEG 生成一张真实可解码的 JPEG，供嗅探、压缩与缩略图全链路使用。
func unitTestJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

// unitCorruptPNG 生成头部合法但数据区损坏的 PNG：类型嗅探可通过，图片解码必然失败，
// 用于触发 CompressImage 失败分支。
func unitCorruptPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 64, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	raw := buf.Bytes()
	corrupt := append([]byte(nil), raw...)
	mid := len(corrupt) / 2
	corrupt[mid] ^= 0xFF
	corrupt[mid+1] ^= 0xFF
	require.NotEqual(t, raw, corrupt)
	return corrupt
}

// unitTestMP4 构造仅含 ftyp 头的最小 MP4 魔数：嗅探识别为 video/mp4，上传不做转码。
func unitTestMP4() []byte {
	head := []byte{
		0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm',
		0x00, 0x00, 0x02, 0x00, 'i', 's', 'o', 'm', 'i', 's', 'o', '2',
		'a', 'v', 'c', '1', 'm', 'p', '4', '1',
	}
	return append(head, bytes.Repeat([]byte{0x00}, 64)...)
}

func unitUploadConfig() config.Config {
	cfg := config.Config{
		S3Storage: storage.Config{
			Bucket: "unit-bucket",
		},
		Upload: config.UploadConf{
			MaxImageSize:      10 * 1024 * 1024,
			MaxVideoSize:      10 * 1024 * 1024,
			DefaultQuality:    85,
			ThumbnailLongSide: 256,
		},
	}
	return cfg
}

func unitSvcCtx(cfg config.Config, mediaModel model.MediaModel, commandModel model.MediaCommandModel, store *unitObjectStorage) *svc.ServiceContext {
	return &svc.ServiceContext{
		Config:            cfg,
		MediaModel:        mediaModel,
		MediaCommandModel: commandModel,
		Storage:           store,
	}
}

// unitAssertBiz 断言错误为指定业务错误码。
func unitAssertBiz(t *testing.T, err error, expectedCode int) {
	t.Helper()
	require.Error(t, err)
	require.True(t, errx.Is(err, expectedCode),
		"期望错误码 %d，实际错误: %v", expectedCode, err)
}
