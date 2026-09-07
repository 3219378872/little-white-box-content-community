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
	for _, kind := range []string{"image", "video"} {
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
			} else {
				stream := unitVideoStreamFromBytes(ctx, 1, "video.mp4", "committed-video", unitTestMP4(), 64)
				stream.sendErr = sendErr
				err = NewUploadVideoLogic(ctx, svcCtx).UploadVideo(stream)
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
