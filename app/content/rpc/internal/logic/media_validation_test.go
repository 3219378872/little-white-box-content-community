package logic

import (
	"context"
	"testing"

	"esx/app/media/rpc/mediaservice"
	mediapb "esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"esx/pkg/errx"

	"github.com/stretchr/testify/assert"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc"
)

type fakeMediaValidator struct {
	mediaservice.MediaService
	response *mediapb.BatchGetMediaResp
	err      error
}

func (f *fakeMediaValidator) BatchGetMedia(_ context.Context, _ *mediapb.BatchGetMediaReq, _ ...grpc.CallOption) (*mediapb.BatchGetMediaResp, error) {
	return f.response, f.err
}

func completedMedia(id, userID int64) *mediapb.MediaInfo {
	return &mediapb.MediaInfo{Id: id, UserId: userID, Status: 1}
}

func TestValidatePostMedia(t *testing.T) {
	logger := logx.WithContext(context.Background())

	t.Run("无媒体ID直接通过", func(t *testing.T) {
		urls, err := validatePostMedia(context.Background(), logger, nil, 1, nil)
		assert.NoError(t, err)
		assert.Empty(t, urls)
	})

	t.Run("媒体归属与完成校验通过", func(t *testing.T) {
		fake := &fakeMediaValidator{response: &mediapb.BatchGetMediaResp{
			Medias: []*mediapb.MediaInfo{completedMedia(10, 1), completedMedia(11, 1)},
		}}
		urls, err := validatePostMedia(context.Background(), logger, fake, 1, []int64{10, 11})
		assert.NoError(t, err)
		assert.Len(t, urls, 2)
	})

	t.Run("引用他人媒体被拒绝", func(t *testing.T) {
		fake := &fakeMediaValidator{response: &mediapb.BatchGetMediaResp{
			Medias: []*mediapb.MediaInfo{completedMedia(10, 99)},
		}}
		_, err := validatePostMedia(context.Background(), logger, fake, 1, []int64{10})
		assert.True(t, errx.Is(err, errx.ParamError))
	})

	t.Run("未完成媒体被拒绝", func(t *testing.T) {
		fake := &fakeMediaValidator{response: &mediapb.BatchGetMediaResp{
			Medias: []*mediapb.MediaInfo{{Id: 10, UserId: 1, Status: 0}},
		}}
		_, err := validatePostMedia(context.Background(), logger, fake, 1, []int64{10})
		assert.True(t, errx.Is(err, errx.ParamError))
	})

	t.Run("引用不存在的媒体被拒绝", func(t *testing.T) {
		fake := &fakeMediaValidator{response: &mediapb.BatchGetMediaResp{
			Medias: []*mediapb.MediaInfo{completedMedia(10, 1)},
		}}
		_, err := validatePostMedia(context.Background(), logger, fake, 1, []int64{10, 999})
		assert.True(t, errx.Is(err, errx.ParamError))
	})

	t.Run("媒体服务不可用返回不可用", func(t *testing.T) {
		_, err := validatePostMedia(context.Background(), logger, nil, 1, []int64{10})
		assert.True(t, errx.Is(err, errx.ServiceUnavailable))
	})

	t.Run("媒体RPC失败返回不可用", func(t *testing.T) {
		fake := &fakeMediaValidator{err: context.DeadlineExceeded}
		_, err := validatePostMedia(context.Background(), logger, fake, 1, []int64{10})
		assert.True(t, errx.Is(err, errx.ServiceUnavailable))
	})
}
