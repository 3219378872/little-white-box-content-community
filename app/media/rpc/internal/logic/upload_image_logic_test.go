package logic

import (
	"context"
	"esx/app/media/rpc/internal/model"
	"esx/pkg/errx"
	"esx/pkg/idempotencyx"
	"esx/pkg/outboxx"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadImageLogic_Success(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	data := unitTestJPEG(t, 64, 48)
	store := &unitObjectStorage{}

	var (
		capturedRow  *model.Media
		capturedIdem idempotencyx.IdempotencyRecord
	)
	commandModel := &fakeMediaCommandModel{
		createMediaFn: func(ctx context.Context, media *model.Media, idem idempotencyx.IdempotencyRecord) (model.MediaCommandResult, error) {
			capturedRow = media
			capturedIdem = idem
			return model.MediaCommandResult{MediaID: media.Id, Created: true}, nil
		},
	}

	stream := unitImageStreamFromBytes(ctx, 4001, "hello.jpg", "idem-img-1", data, 256)
	l := NewUploadImageLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, commandModel, store))
	require.NoError(t, l.UploadImage(stream))

	// 响应：压缩后尺寸、原始与缩略 URL。
	require.NotNil(t, stream.resp)
	require.NotNil(t, stream.resp.Media)
	assert.Greater(t, stream.resp.Media.Id, int64(0))
	assert.Equal(t, int32(64), stream.resp.Media.Width)
	assert.Equal(t, int32(48), stream.resp.Media.Height)
	assert.Contains(t, stream.resp.Media.Url, "original/")
	assert.Contains(t, stream.resp.Media.ThumbnailUrl, "thumb/")
	assert.Positive(t, stream.resp.Media.FileSize)

	// 对象存储：原图 + 缩略图各一次，均为 JPEG 且非空。
	require.Len(t, store.putCalls, 2)
	assert.Contains(t, store.putCalls[0].objectKey, "original/")
	assert.Equal(t, "image/jpeg", store.putCalls[0].contentType)
	assert.Contains(t, store.putCalls[1].objectKey, "thumb/")
	assert.Equal(t, "image/jpeg", store.putCalls[1].contentType)
	assert.Empty(t, store.deleteKeys)

	// 落库行：类型、存储后端、桶、状态与 URL 一致。
	require.NotNil(t, capturedRow)
	assert.Equal(t, "image", capturedRow.FileType)
	assert.True(t, capturedRow.MimeType.Valid)
	assert.Equal(t, "image/jpeg", capturedRow.MimeType.String)
	assert.Equal(t, int64(storageTypeSeaweedFS), capturedRow.StorageType)
	assert.Equal(t, "unit-bucket", capturedRow.Bucket.String)
	assert.True(t, capturedRow.ObjectKey.Valid)
	assert.Equal(t, store.putCalls[0].objectKey, capturedRow.ObjectKey.String)
	assert.Equal(t, store.BuildPublicURL(store.putCalls[0].objectKey), capturedRow.Url)
	assert.Equal(t, store.BuildPublicURL(store.putCalls[1].objectKey), capturedRow.ThumbnailUrl.String)
	assert.Equal(t, int64(1), capturedRow.Status)
	assert.Equal(t, int64(4001), capturedRow.UserId)
	assert.Equal(t, "hello.jpg", capturedRow.FileName)

	// 幂等记录：scope 固定、键透传、命令哈希覆盖文件内容。
	assert.Equal(t, "media:upload", capturedIdem.Scope)
	assert.Equal(t, int64(4001), capturedIdem.UserID)
	assert.Equal(t, "idem-img-1", capturedIdem.Key)
	assert.NotEmpty(t, capturedIdem.CommandHash)
}

func TestUploadImageLogic_IdempotentRetryReturnsExistingAndCleansOrphans(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	data := unitTestJPEG(t, 32, 24)
	store := &unitObjectStorage{}
	existing := &model.Media{
		Id:        42,
		UserId:    4002,
		FileName:  "old.jpg",
		FileType:  "image",
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

	stream := unitImageStreamFromBytes(ctx, 4002, "retry.jpg", "idem-retry-1", data, 256)
	l := NewUploadImageLogic(ctx, unitSvcCtx(unitUploadConfig(), mediaModel, commandModel, store))
	require.NoError(t, l.UploadImage(stream))

	// 返回已有记录而不是本次上传的行。
	require.NotNil(t, stream.resp)
	require.NotNil(t, stream.resp.Media)
	assert.Equal(t, existing.Id, stream.resp.Media.Id)
	assert.Equal(t, "old.jpg", stream.resp.Media.FileName)
	assert.Equal(t, "http://storage.test/bucket/original/old", stream.resp.Media.Url)

	// 本次上传产生的两个孤儿对象被 best-effort 删除。
	require.Len(t, store.putCalls, 2)
	require.Len(t, store.deleteKeys, 2)
	assert.ElementsMatch(t, []string{store.putCalls[0].objectKey, store.putCalls[1].objectKey}, store.deleteKeys)
}

func TestUploadImageLogic_IdempotentRetryToleratesOrphanDeleteFailure(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	data := unitTestJPEG(t, 16, 16)
	store := &unitObjectStorage{failDeletes: true}
	var cleanupEvents []outboxx.Event
	commandModel := &fakeMediaCommandModel{
		createMediaFn: func(ctx context.Context, media *model.Media, idem idempotencyx.IdempotencyRecord) (model.MediaCommandResult, error) {
			return model.MediaCommandResult{MediaID: 43, Created: false}, nil
		},
		enqueueObjectCleanupFn: func(ctx context.Context, event outboxx.Event) error {
			cleanupEvents = append(cleanupEvents, event)
			return nil
		},
	}
	mediaModel := &fakeMediaModel{
		findOneFn: func(ctx context.Context, id int64) (*model.Media, error) {
			return &model.Media{Id: id, FileName: "kept.jpg", FileType: "image", Status: 1, CreatedAt: time.Now()}, nil
		},
	}

	stream := unitImageStreamFromBytes(ctx, 4003, "again.jpg", "idem-retry-2", data, 256)
	l := NewUploadImageLogic(ctx, unitSvcCtx(unitUploadConfig(), mediaModel, commandModel, store))
	// 孤儿对象删除失败不影响已提交的成功响应，并进入可靠 outbox。
	require.NoError(t, l.UploadImage(stream))
	require.NotNil(t, stream.resp)
	assert.Equal(t, "kept.jpg", stream.resp.Media.FileName)
	require.Len(t, cleanupEvents, 2)
	for _, queued := range cleanupEvents {
		assert.NotEmpty(t, queued.Payload)
		assert.NotEmpty(t, queued.Key)
	}
}

func TestUploadImageLogic_RetryFindExistingFailed(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	data := unitTestJPEG(t, 16, 16)
	store := &unitObjectStorage{}
	commandModel := &fakeMediaCommandModel{
		createMediaFn: func(ctx context.Context, media *model.Media, idem idempotencyx.IdempotencyRecord) (model.MediaCommandResult, error) {
			return model.MediaCommandResult{MediaID: 44, Created: false}, nil
		},
	}
	mediaModel := &fakeMediaModel{
		findOneFn: func(ctx context.Context, id int64) (*model.Media, error) {
			return nil, assert.AnError
		},
	}

	stream := unitImageStreamFromBytes(ctx, 4004, "gone.jpg", "idem-retry-3", data, 256)
	l := NewUploadImageLogic(ctx, unitSvcCtx(unitUploadConfig(), mediaModel, commandModel, store))
	unitAssertBiz(t, l.UploadImage(stream), errx.SystemError)
}

func TestUploadImageLogic_UserIdInvalid(t *testing.T) {
	ctx := context.Background()
	store := &unitObjectStorage{}
	stream := unitImageStreamFromBytes(ctx, 0, "anon.jpg", "idem-u0", []byte("abc"), 256)
	l := NewUploadImageLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadImage(stream), errx.ParamError)
	assert.Empty(t, store.putCalls)
}

func TestUploadImageLogic_IdempotencyKeyTooLong(t *testing.T) {
	ctx := context.Background()
	store := &unitObjectStorage{}
	longKey := strings.Repeat("k", 129)
	stream := unitImageStreamFromBytes(ctx, 4005, "long-key.jpg", longKey, []byte("abc"), 256)
	l := NewUploadImageLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadImage(stream), errx.ParamError)
	assert.Empty(t, store.putCalls)
}

func TestUploadImageLogic_TypeNotAllowed(t *testing.T) {
	ctx := context.Background()
	data := append([]byte{0x25, 0x50, 0x44, 0x46, 0x2D, 0x31, 0x2E, 0x34}, make([]byte, 1024)...)
	store := &unitObjectStorage{}
	stream := unitImageStreamFromBytes(ctx, 4006, "doc.pdf", "idem-pdf", data, 512)
	l := NewUploadImageLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadImage(stream), errx.FileTypeNotAllowed)
	assert.Empty(t, store.putCalls)
}

func TestUploadImageLogic_CompressFailed(t *testing.T) {
	ctx := context.Background()
	data := unitCorruptPNG(t)
	store := &unitObjectStorage{}
	stream := unitImageStreamFromBytes(ctx, 4007, "broken.png", "idem-png", data, 512)
	l := NewUploadImageLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadImage(stream), errx.MediaProcessFailed)
	assert.Empty(t, store.putCalls)
}

func TestUploadImageLogic_RejectsOversizedPixelDimensionsBeforeDecode(t *testing.T) {
	ctx := context.Background()
	data := unitOversizedPNGHeader(5001, 5000)
	store := &unitObjectStorage{}
	stream := unitImageStreamFromBytes(ctx, 4007, "oversized.png", "idem-pixels", data, 512)
	l := NewUploadImageLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadImage(stream), errx.FileTooLarge)
	assert.Empty(t, store.putCalls)
}

func TestUploadImageLogic_PutOriginalFailed(t *testing.T) {
	ctx := context.Background()
	data := unitTestJPEG(t, 16, 16)
	store := &unitObjectStorage{putErrOn: 1}
	stream := unitImageStreamFromBytes(ctx, 4008, "fail-original.jpg", "idem-put1", data, 256)
	l := NewUploadImageLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadImage(stream), errx.UploadFailed)
	assert.Empty(t, store.putCalls)
}

func TestUploadImageLogic_PutThumbnailFailed(t *testing.T) {
	ctx := context.Background()
	data := unitTestJPEG(t, 16, 16)
	store := &unitObjectStorage{putErrOn: 2}
	stream := unitImageStreamFromBytes(ctx, 4009, "fail-thumb.jpg", "idem-put2", data, 256)
	l := NewUploadImageLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadImage(stream), errx.UploadFailed)
	// 原图已上传成功、缩略图失败：立即补偿删除原图。
	assert.Len(t, store.putCalls, 1)
	assert.Equal(t, store.putCalls[0].objectKey, store.deleteKeys[0])
}

func TestUploadImageLogic_CreateMediaIdempotencyConflict(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	data := unitTestJPEG(t, 16, 16)
	store := &unitObjectStorage{}
	commandModel := &fakeMediaCommandModel{
		createMediaFn: func(ctx context.Context, media *model.Media, idem idempotencyx.IdempotencyRecord) (model.MediaCommandResult, error) {
			return model.MediaCommandResult{}, idempotencyx.ErrIdempotencyConflict
		},
	}
	stream := unitImageStreamFromBytes(ctx, 4010, "conflict.jpg", "idem-conflict", data, 256)
	l := NewUploadImageLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, commandModel, store))
	unitAssertBiz(t, l.UploadImage(stream), errx.IdempotencyConflict)
	// 上传已完成但落库被拒：两个未提交对象都立即补偿删除。
	assert.Len(t, store.putCalls, 2)
	assert.Len(t, store.deleteKeys, 2)
}

func TestUploadImageLogic_CreateMediaUnexpectedError(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	data := unitTestJPEG(t, 16, 16)
	store := &unitObjectStorage{}
	commandModel := &fakeMediaCommandModel{
		createMediaFn: func(ctx context.Context, media *model.Media, idem idempotencyx.IdempotencyRecord) (model.MediaCommandResult, error) {
			return model.MediaCommandResult{}, assert.AnError
		},
	}
	stream := unitImageStreamFromBytes(ctx, 4011, "dberr.jpg", "idem-dberr", data, 256)
	l := NewUploadImageLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, commandModel, store))
	unitAssertBiz(t, l.UploadImage(stream), errx.SystemError)
	assert.Len(t, store.deleteKeys, 2)
}

func TestUploadImageLogic_CommandModelMissing(t *testing.T) {
	unitInitSnowflake(t)
	ctx := context.Background()
	data := unitTestJPEG(t, 16, 16)
	store := &unitObjectStorage{}
	stream := unitImageStreamFromBytes(ctx, 4012, "nomodel.jpg", "idem-nomodel", data, 256)
	l := NewUploadImageLogic(ctx, unitSvcCtx(unitUploadConfig(), &fakeMediaModel{}, nil, store))
	unitAssertBiz(t, l.UploadImage(stream), errx.SystemError)
	assert.Empty(t, store.putCalls)
}

func TestUploadImageLogic_TempSinkUnavailable(t *testing.T) {
	ctx := context.Background()
	cfg := unitUploadConfig()
	cfg.Upload.MaxImageSize = 0 // 非法 limit：NewTempSink 直接失败。
	store := &unitObjectStorage{}
	stream := unitImageStreamFromBytes(ctx, 4013, "nosink.jpg", "idem-nosink", []byte("abc"), 256)
	l := NewUploadImageLogic(ctx, unitSvcCtx(cfg, &fakeMediaModel{}, &fakeMediaCommandModel{}, store))
	unitAssertBiz(t, l.UploadImage(stream), errx.SystemError)
	assert.Empty(t, store.putCalls)
}
