package logic

import (
	"context"
	"esx/app/content/rpc/internal/svc"
	pb "esx/kitex_gen/content"
	"testing"

	"github.com/cloudwego/kitex/client/callopt"

	"esx/app/media/rpc/mediaservice"
	mediapb "esx/kitex_gen/media"
	"esx/pkg/errx"

	logx "esx/pkg/logging"

	"github.com/stretchr/testify/assert"
)

type fakeMediaValidator struct {
	mediaservice.MediaService
	response *mediapb.BatchGetMediaResp
	err      error
}

func (f *fakeMediaValidator) BatchGetMedia(_ context.Context, _ *mediapb.BatchGetMediaReq, _ ...callopt.Option) (*mediapb.BatchGetMediaResp, error) {
	return f.response, f.err
}

func completedMedia(id, userID int64) *mediapb.MediaInfo {
	return &mediapb.MediaInfo{Id: id, UserId: userID, Status: 1, FileType: "image", Url: "https://media.example/image"}
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

func TestPostMediaRejectsWrongKindAndEmptyURL(t *testing.T) {
	for _, tt := range []struct{ kind, url string }{{"audio", "audio.mp3"}, {"video", "video.mp4"}, {"image", ""}, {"image", " \n\t"}, {"", "image.jpg"}} {
		t.Run(tt.kind+tt.url, func(t *testing.T) {
			service := &fakeMediaValidator{response: &mediapb.BatchGetMediaResp{Medias: []*mediapb.MediaInfo{{Id: 10, UserId: 1, Status: 1, FileType: tt.kind, Url: tt.url}}}}
			ctx := context.Background()
			_, err := validatePostMedia(ctx, logx.WithContext(ctx), service, 1, []int64{10})
			assert.True(t, errx.Is(err, errx.ParamError))
		})
	}
}

func TestPostCreateAndUpdateRejectInvalidImageMedia(t *testing.T) {
	for _, tt := range []struct{ kind, url string }{{"audio", "audio.mp3"}, {"video", "video.mp4"}, {"image", ""}} {
		service := &fakeMediaValidator{response: &mediapb.BatchGetMediaResp{Medias: []*mediapb.MediaInfo{{Id: 10, UserId: 1, Status: 1, FileType: tt.kind, Url: tt.url}}}}
		ctx := context.Background()
		sc := &svc.ServiceContext{MediaService: service}
		created, err := NewCreatePostLogic(ctx, sc).CreatePost(&pb.CreatePostReq{AuthorId: 1, Title: "title", Content: "body", Status: 1, MediaIds: []int64{10}})
		assert.Nil(t, created)
		assert.True(t, errx.Is(err, errx.ParamError))
		updated, err := NewUpdatePostLogic(ctx, sc).UpdatePost(&pb.UpdatePostReq{PostId: 5, AuthorId: 1, ExpectedRevision: 3, MediaIds: []int64{10}})
		assert.Nil(t, updated)
		assert.True(t, errx.Is(err, errx.ParamError))
	}
}
