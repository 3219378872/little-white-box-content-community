package logic

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	pb "esx/kitex_gen/review"
	"esx/pkg/errx"
)

// ADS-031：政策版本或生效种子集合变化产生新回扫代次；版本未被 worker 激活时不回扫。
func TestRescanGenerationTracksPolicyAndSeeds(t *testing.T) {
	svcCtx, fs := newFixture()
	logic := NewGetRescanGenerationLogic(context.Background(), svcCtx)

	resp, err := logic.GetRescanGeneration(&pb.GetRescanGenerationReq{})
	require.NoError(t, err)
	require.False(t, resp.GetReady())
	require.Empty(t, resp.GetGeneration())

	fs.activated, fs.activatedAt = true, 1_000
	resp, err = logic.GetRescanGeneration(&pb.GetRescanGenerationReq{})
	require.NoError(t, err)
	require.True(t, resp.GetReady())
	require.Equal(t, "demo-v1+seeds@0", resp.GetGeneration())
	require.Equal(t, int64(1_000), resp.GetEffectiveSinceMs())

	fs.seedChanges, fs.seedChangedAt = 3, 5_000
	resp, err = logic.GetRescanGeneration(&pb.GetRescanGenerationReq{})
	require.NoError(t, err)
	require.Equal(t, "demo-v1+seeds@3", resp.GetGeneration())
	require.Equal(t, 5_000+SeedIndexLag.Milliseconds(), resp.GetEffectiveSinceMs())
}

func TestRescanGenerationStoreFailures(t *testing.T) {
	svcCtx, fs := newFixture()
	fs.activationErr = errBoom
	_, err := NewGetRescanGenerationLogic(context.Background(), svcCtx).GetRescanGeneration(&pb.GetRescanGenerationReq{})
	requireCode(t, err, errx.SystemError)

	fs.activationErr, fs.activated, fs.seedGenErr = nil, true, errBoom
	_, err = NewGetRescanGenerationLogic(context.Background(), svcCtx).GetRescanGeneration(&pb.GetRescanGenerationReq{})
	requireCode(t, err, errx.SystemError)
}
