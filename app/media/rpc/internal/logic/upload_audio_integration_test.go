//go:build integration

package logic

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUploadAudioPersistenceAndReplay(t *testing.T) {
	ctx := context.Background()
	first := unitAudioStreamFromBytes(ctx, 50123, "voice.wav", "audio-integration-replay", unitTestWAV(), 16)
	require.NoError(t, NewUploadAudioLogic(ctx, testSvcCtx).UploadAudio(first))
	retry := unitAudioStreamFromBytes(ctx, 50123, "voice.wav", "audio-integration-replay", unitTestWAV(), 16)
	require.NoError(t, NewUploadAudioLogic(ctx, testSvcCtx).UploadAudio(retry))
	require.Equal(t, first.resp.Media.Id, retry.resp.Media.Id)
	stored, err := testSvcCtx.MediaModel.FindOne(ctx, first.resp.Media.Id)
	require.NoError(t, err)
	require.Equal(t, "audio", stored.FileType)
	require.Equal(t, int64(1), stored.Status)
}
