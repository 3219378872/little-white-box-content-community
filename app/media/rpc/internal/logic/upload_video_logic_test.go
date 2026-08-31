package logic

import (
	"context"
	"esx/app/media/rpc/internal/model"
	"esx/pkg/errx"
	"esx/pkg/idempotencyx"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadVideoLogic_Success(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	data := unitTestMP4()
	store := &unitObjectStorage{}

	var capturedRow *model.Media
	commandModel := &fakeMediaCommandModel{
		createMediaFn: func(ctx context.Context, media *model.Media, idem idempotencyx.IdempotencyRecord) (model.MediaCommandResult, error) {
			capturedRow = media
			return model.MediaCommandResult{MediaID: media.Id, Created: true}, nil
		},
	}

	stream := unitVideoStreamFromBytes(ctx, 5001, "clip.mp4", "idem-vid-1", data, 64)
	l := NewUploadVideoLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, commandModel, store))
	require.NoError(t, l.UploadVideo(stream))

	require.NotNil(t, stream.resp)
	require.NotNil(t, stream.resp.Media)
	assert.Greater(t, stream.resp.Media.Id, int64(0))
	assert.Equal(t, int64(len(data)), stream.resp.Media.FileSize)
	assert.Contains(t, stream.resp.Media.Url, "original/")
	assert.Empty(t, stream.resp.Media.ThumbnailUrl)

	// 视频不转码：仅原图对象一次，MIME 来自嗅探结果。
	require.Len(t, store.putCalls, 1)
	assert.Contains(t, store.putCalls[0].objectKey, "original/")
	assert.Contains(t, store.putCalls[0].objectKey, ".mp4")
	assert.Equal(t, "video/mp4", store.putCalls[0].contentType)

	require.NotNil(t, capturedRow)
	assert.Equal(t, "video", capturedRow.FileType)
	assert.True(t, capturedRow.MimeType.Valid)
	assert.Equal(t, "video/mp4", capturedRow.MimeType.String)
	assert.Equal(t, int64(storageTypeSeaweedFS), capturedRow.StorageType)
	assert.Equal(t, "unit-bucket", capturedRow.Bucket.String)
	assert.Equal(t, store.BuildPublicURL(store.putCalls[0].objectKey), capturedRow.Url)
	assert.False(t, capturedRow.ThumbnailUrl.Valid)
	assert.False(t, capturedRow.Width.Valid)
	assert.False(t, capturedRow.Height.Valid)
	assert.Equal(t, int64(len(data)), capturedRow.FileSize)
	assert.Equal(t, int64(1), capturedRow.Status)
	assert.Equal(t, int64(5001), capturedRow.UserId)
}

func TestUploadVideoLogic_IdempotentRetryReturnsExistingAndCleansOrphans(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	data := unitTestMP4()
	store := &unitObjectStorage{}
	existing := &model.Media{
		Id:        52,
		UserId:    5002,
		FileName:  "old.mp4",
		FileType:  "video",
		Url:       "http://storage.test/bucket/original/old",
		Status:    1,
		CreatedAt: time.Now(),
	}
	commandModel := &fakeMediaCommandModel{
		createMediaFn: func(ctx context.Context, media *model.Media, idem idempotencyx.IdempotencyRecord) (model.MediaCommandResult, error) {
			return model.MediaCommandResult{MediaID: existing.Id, Created: false}, nil
		},
	}
	mediaModel := &fakeMediaModel{
		findOneFn: func(ctx context.Context, id int64) (*model.Media, error) {
			assert.Equal(t, existing.Id, id)
			return existing, nil
		},
	}

	stream := unitVideoStreamFromBytes(ctx, 5002, "retry.mp4", "idem-vid-retry", data, 64)
	l := NewUploadVideoLogic(ctx, unitSvcCtx(unitUploadConfig(), mediaModel, commandModel, store))
	require.NoError(t, l.UploadVideo(stream))

	require.NotNil(t, stream.resp)
	require.NotNil(t, stream.resp.Media)
	assert.Equal(t, existing.Id, stream.resp.Media.Id)
	assert.Equal(t, "old.mp4", stream.resp.Media.FileName)

	// 仅一个本次上传的孤儿对象被删除。
	require.Len(t, store.putCalls, 1)
	require.Len(t, store.deleteKeys, 1)
	assert.Equal(t, store.putCalls[0].objectKey, store.deleteKeys[0])
}

func TestUploadVideoLogic_UserIdInvalid(t *testing.T) {
	ctx := context.Background()
	store := &unitObjectStorage{}
	stream := unitVideoStreamFromBytes(ctx, -1, "anon.mp4", "idem-vu0", unitTestMP4(), 64)
	l := NewUploadVideoLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadVideo(stream), errx.ParamError)
	assert.Empty(t, store.putCalls)
}

func TestUploadVideoLogic_IdempotencyKeyTooLong(t *testing.T) {
	ctx := context.Background()
	store := &unitObjectStorage{}
	stream := unitVideoStreamFromBytes(ctx, 5003, "long-key.mp4", strings.Repeat("k", 200), unitTestMP4(), 64)
	l := NewUploadVideoLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadVideo(stream), errx.ParamError)
	assert.Empty(t, store.putCalls)
}

func TestUploadVideoLogic_TypeNotAllowed(t *testing.T) {
	ctx := context.Background()
	data := append([]byte{0x25, 0x50, 0x44, 0x46, 0x2D, 0x31, 0x2E, 0x34}, make([]byte, 512)...)
	store := &unitObjectStorage{}
	stream := unitVideoStreamFromBytes(ctx, 5004, "doc.pdf", "idem-vpdf", data, 256)
	l := NewUploadVideoLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadVideo(stream), errx.FileTypeNotAllowed)
	assert.Empty(t, store.putCalls)
}

func TestUploadVideoLogic_PutFailed(t *testing.T) {
	ctx := context.Background()
	store := &unitObjectStorage{putErrOn: 1}
	stream := unitVideoStreamFromBytes(ctx, 5005, "fail.mp4", "idem-vput", unitTestMP4(), 64)
	l := NewUploadVideoLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadVideo(stream), errx.UploadFailed)
	assert.Empty(t, store.putCalls)
}

func TestUploadVideoLogic_CreateMediaIdempotencyConflict(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	store := &unitObjectStorage{}
	commandModel := &fakeMediaCommandModel{
		createMediaFn: func(ctx context.Context, media *model.Media, idem idempotencyx.IdempotencyRecord) (model.MediaCommandResult, error) {
			return model.MediaCommandResult{}, idempotencyx.ErrIdempotencyConflict
		},
	}
	stream := unitVideoStreamFromBytes(ctx, 5006, "conflict.mp4", "idem-vconflict", unitTestMP4(), 64)
	l := NewUploadVideoLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, commandModel, store))
	unitAssertBiz(t, l.UploadVideo(stream), errx.IdempotencyConflict)
	assert.Len(t, store.putCalls, 1)
	assert.Len(t, store.deleteKeys, 1)
}

func TestUploadVideoLogic_CreateMediaUnexpectedError(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	store := &unitObjectStorage{}
	commandModel := &fakeMediaCommandModel{
		createMediaFn: func(ctx context.Context, media *model.Media, idem idempotencyx.IdempotencyRecord) (model.MediaCommandResult, error) {
			return model.MediaCommandResult{}, assert.AnError
		},
	}
	stream := unitVideoStreamFromBytes(ctx, 5007, "dberr.mp4", "idem-vdberr", unitTestMP4(), 64)
	l := NewUploadVideoLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, commandModel, store))
	unitAssertBiz(t, l.UploadVideo(stream), errx.SystemError)
	assert.Len(t, store.deleteKeys, 1)
}

func TestUploadVideoLogic_CommandModelMissing(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	store := &unitObjectStorage{}
	stream := unitVideoStreamFromBytes(ctx, 5008, "nomodel.mp4", "idem-vnomodel", unitTestMP4(), 64)
	l := NewUploadVideoLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, nil, store))
	unitAssertBiz(t, l.UploadVideo(stream), errx.SystemError)
	assert.Empty(t, store.putCalls)
}

func TestUploadVideoLogic_TempSinkUnavailable(t *testing.T) {
	ctx := context.Background()
	cfg := unitUploadConfig()
	cfg.Upload.MaxVideoSize = 0 // 非法 limit：NewTempSink 直接失败。
	store := &unitObjectStorage{}
	stream := unitVideoStreamFromBytes(ctx, 5009, "nosink.mp4", "idem-vnosink", unitTestMP4(), 64)
	l := NewUploadVideoLogic(ctx, unitSvcCtx(cfg, &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadVideo(stream), errx.SystemError)
	assert.Empty(t, store.putCalls)
}
