package logic

import (
	"context"
	"errors"
	"os"
	"testing"

	"esx/app/media/rpc/internal/model"
	"esx/pkg/idempotencyx"

	"github.com/stretchr/testify/require"
)

func TestCommittedUploadSurvivesResponseFailure(t *testing.T) {
	unitInitSnowflake(t)
	for _, kind := range []string{"image", "video", "audio"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			sendErr := errors.New("client disconnected after commit")
			storage := &unitObjectStorage{}
			command := &fakeMediaCommandModel{createMediaFn: func(_ context.Context, row *model.Media, _ idempotencyx.IdempotencyRecord) (model.MediaCommandResult, error) {
				return model.MediaCommandResult{MediaID: row.Id, Created: true}, nil
			}}
			config := unitUploadConfig()
			config.Upload.TempDir = t.TempDir()
			svcCtx := unitSvcCtx(config, &fakeMediaModel{}, command, storage)
			var err error
			if kind == "image" {
				stream := unitImageStreamFromBytes(ctx, 1, "image.jpg", "committed-image", unitTestJPEG(t, 32, 24), 256)
				stream.sendErr = sendErr
				err = NewUploadImageLogic(ctx, svcCtx).UploadImage(stream)
				require.Len(t, storage.putCalls, 2)
			} else if kind == "video" {
				stream := unitVideoStreamFromBytes(ctx, 1, "video.mp4", "committed-video", unitTestMP4(), 64)
				stream.sendErr = sendErr
				err = NewUploadVideoLogic(ctx, svcCtx).UploadVideo(stream)
				require.Len(t, storage.putCalls, 1)
			} else {
				stream := unitAudioStreamFromBytes(ctx, 1, "audio.wav", "committed-audio", unitTestWAV(), 64)
				stream.sendErr = sendErr
				err = NewUploadAudioLogic(ctx, svcCtx).UploadAudio(stream)
				require.Len(t, storage.putCalls, 1)
			}
			require.ErrorIs(t, err, sendErr)
			require.Empty(t, storage.deleteKeys)
			files, err := os.ReadDir(config.Upload.TempDir)
			require.NoError(t, err)
			require.Empty(t, files)
		})
	}
}

// An adapter that cannot classify a failure must opt into retention by default;
// the zero value must never grant permission to delete possibly committed data.
func TestUnclassifiedUploadFailureRetainsObjects(t *testing.T) {
	unitInitSnowflake(t)
	for _, kind := range []string{"image", "video", "audio"} {
		t.Run(kind, func(t *testing.T) {
			storage := &unitObjectStorage{}
			command := &fakeMediaCommandModel{createMediaFn: func(context.Context, *model.Media, idempotencyx.IdempotencyRecord) (model.MediaCommandResult, error) {
				return model.MediaCommandResult{}, errors.New("unclassified persistence failure")
			}}
			cfg := unitUploadConfig()
			cfg.Upload.TempDir = t.TempDir()
			sc := unitSvcCtx(cfg, &fakeMediaModel{}, command, storage)
			_, err := runCommitSafetyUpload(t, context.Background(), sc, kind, "unknown-adapter")
			require.Error(t, err)
			require.NotEmpty(t, storage.putCalls)
			require.Empty(t, storage.deleteKeys)
		})
	}
}
