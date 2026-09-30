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

func TestUploadAudioLogic_Success(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	data := unitTestWAV()
	store := &unitObjectStorage{}

	var capturedRow *model.Media
	commandModel := &fakeMediaCommandModel{
		createMediaFn: func(ctx context.Context, media *model.Media, idem idempotencyx.IdempotencyRecord) (model.MediaCommandResult, error) {
			capturedRow = media
			return model.MediaCommandResult{MediaID: media.Id, Created: true}, nil
		},
	}

	stream := unitAudioStreamFromBytes(ctx, 5001, "clip.wav", "idem-vid-1", data, 64)
	l := NewUploadAudioLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, commandModel, store))
	require.NoError(t, l.UploadAudio(stream))

	require.NotNil(t, stream.resp)
	require.NotNil(t, stream.resp.Media)
	assert.Greater(t, stream.resp.Media.Id, int64(0))
	assert.Equal(t, int64(len(data)), stream.resp.Media.FileSize)
	assert.Contains(t, stream.resp.Media.Url, "original/")
	assert.Empty(t, stream.resp.Media.ThumbnailUrl)

	// 视频不转码：仅原图对象一次，MIME 来自嗅探结果。
	require.Len(t, store.putCalls, 1)
	assert.Contains(t, store.putCalls[0].objectKey, "original/")
	assert.Contains(t, store.putCalls[0].objectKey, ".wav")
	assert.Equal(t, "audio/wav", store.putCalls[0].contentType)

	require.NotNil(t, capturedRow)
	assert.Equal(t, "audio", capturedRow.FileType)
	assert.True(t, capturedRow.MimeType.Valid)
	assert.Equal(t, "audio/wav", capturedRow.MimeType.String)
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

func TestUploadAudioLogic_IdempotentRetryReturnsExistingAndCleansOrphans(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	data := unitTestWAV()
	store := &unitObjectStorage{}
	existing := &model.Media{
		Id:        52,
		UserId:    5002,
		FileName:  "old.wav",
		FileType:  "audio",
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

	stream := unitAudioStreamFromBytes(ctx, 5002, "retry.wav", "idem-vid-retry", data, 64)
	l := NewUploadAudioLogic(ctx, unitSvcCtx(unitUploadConfig(), mediaModel, commandModel, store))
	require.NoError(t, l.UploadAudio(stream))

	require.NotNil(t, stream.resp)
	require.NotNil(t, stream.resp.Media)
	assert.Equal(t, existing.Id, stream.resp.Media.Id)
	assert.Equal(t, "old.wav", stream.resp.Media.FileName)

	// 仅一个本次上传的孤儿对象被删除。
	require.Len(t, store.putCalls, 1)
	require.Len(t, store.deleteKeys, 1)
	assert.Equal(t, store.putCalls[0].objectKey, store.deleteKeys[0])
}

func TestUploadAudioLogic_UserIdInvalid(t *testing.T) {
	ctx := context.Background()
	store := &unitObjectStorage{}
	stream := unitAudioStreamFromBytes(ctx, -1, "anon.wav", "idem-vu0", unitTestWAV(), 64)
	l := NewUploadAudioLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadAudio(stream), errx.ParamError)
	assert.Empty(t, store.putCalls)
}

func TestUploadAudioLogic_IdempotencyKeyTooLong(t *testing.T) {
	ctx := context.Background()
	store := &unitObjectStorage{}
	stream := unitAudioStreamFromBytes(ctx, 5003, "long-key.wav", strings.Repeat("k", 200), unitTestWAV(), 64)
	l := NewUploadAudioLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadAudio(stream), errx.ParamError)
	assert.Empty(t, store.putCalls)
}

func TestUploadAudioLogic_TypeNotAllowed(t *testing.T) {
	ctx := context.Background()
	data := append([]byte{0x25, 0x50, 0x44, 0x46, 0x2D, 0x31, 0x2E, 0x34}, make([]byte, 512)...)
	store := &unitObjectStorage{}
	stream := unitAudioStreamFromBytes(ctx, 5004, "doc.pdf", "idem-vpdf", data, 256)
	l := NewUploadAudioLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadAudio(stream), errx.FileTypeNotAllowed)
	assert.Empty(t, store.putCalls)
}

func TestUploadAudioLogic_PutFailed(t *testing.T) {
	ctx := context.Background()
	store := &unitObjectStorage{putErrOn: 1}
	stream := unitAudioStreamFromBytes(ctx, 5005, "fail.wav", "idem-vput", unitTestWAV(), 64)
	l := NewUploadAudioLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadAudio(stream), errx.UploadFailed)
	assert.Empty(t, store.putCalls)
}

func TestUploadAudioLogic_CreateMediaIdempotencyConflict(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	store := &unitObjectStorage{}
	commandModel := &fakeMediaCommandModel{
		createMediaFn: func(ctx context.Context, media *model.Media, idem idempotencyx.IdempotencyRecord) (model.MediaCommandResult, error) {
			return model.MediaCommandResult{Outcome: model.MediaNotCommitted}, idempotencyx.ErrIdempotencyConflict
		},
	}
	stream := unitAudioStreamFromBytes(ctx, 5006, "conflict.wav", "idem-vconflict", unitTestWAV(), 64)
	l := NewUploadAudioLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, commandModel, store))
	unitAssertBiz(t, l.UploadAudio(stream), errx.IdempotencyConflict)
	assert.Len(t, store.putCalls, 1)
	assert.Len(t, store.deleteKeys, 1)
}

func TestUploadAudioLogic_CreateMediaUnexpectedError(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	store := &unitObjectStorage{}
	commandModel := &fakeMediaCommandModel{
		createMediaFn: func(ctx context.Context, media *model.Media, idem idempotencyx.IdempotencyRecord) (model.MediaCommandResult, error) {
			return model.MediaCommandResult{Outcome: model.MediaNotCommitted}, assert.AnError
		},
	}
	stream := unitAudioStreamFromBytes(ctx, 5007, "dberr.wav", "idem-vdberr", unitTestWAV(), 64)
	l := NewUploadAudioLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, commandModel, store))
	unitAssertBiz(t, l.UploadAudio(stream), errx.SystemError)
	assert.Len(t, store.deleteKeys, 1)
}

func TestUploadAudioLogic_CommandModelMissing(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	store := &unitObjectStorage{}
	stream := unitAudioStreamFromBytes(ctx, 5008, "nomodel.wav", "idem-vnomodel", unitTestWAV(), 64)
	l := NewUploadAudioLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, nil, store))
	unitAssertBiz(t, l.UploadAudio(stream), errx.SystemError)
	assert.Empty(t, store.putCalls)
}

func TestUploadAudioLogic_TempSinkUnavailable(t *testing.T) {
	ctx := context.Background()
	cfg := unitUploadConfig()
	cfg.Upload.MaxAudioSize = 0 // 非法 limit：NewTempSink 直接失败。
	store := &unitObjectStorage{}
	stream := unitAudioStreamFromBytes(ctx, 5009, "nosink.wav", "idem-vnosink", unitTestWAV(), 64)
	l := NewUploadAudioLogic(ctx, unitSvcCtx(cfg, &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadAudio(stream), errx.SystemError)
	assert.Empty(t, store.putCalls)
}
