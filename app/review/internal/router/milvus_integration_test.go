//go:build integration

package router

import (
	"context"
	"testing"
	"time"

	"esx/pkg/testutil"

	"github.com/stretchr/testify/require"
)

// DES-review-platform W3 风险：先验证种子集合的插入、删除与按向量检索。
func TestMilvusSeedIndexInsertSearchDelete(t *testing.T) {
	env := testutil.SetupMilvusEnv(t)
	defer env.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	index, err := DialMilvus(ctx, MilvusConfig{Address: env.Address, Collection: "review_seed_it", Dim: 4})
	require.NoError(t, err)
	defer func() { _ = index.Close() }()

	require.NoError(t, index.Insert(ctx, 1, "US", "MISLEADING.CLAIM", []float32{1, 0, 0, 0}))
	require.NoError(t, index.Insert(ctx, 2, "US", "CONTENT.VIOLENCE", []float32{0, 1, 0, 0}))
	require.NoError(t, index.Insert(ctx, 3, "DE", "MISLEADING.CLAIM", []float32{1, 0, 0, 0}))
	require.NoError(t, index.Insert(ctx, 1, "US", "MISLEADING.CLAIM", []float32{1, 0, 0, 0})) // 幂等重写

	hits, err := index.Search(ctx, normalize([]float32{0.9, 0.1, 0, 0}), "US", 5)
	require.NoError(t, err)
	require.Len(t, hits, 2)
	require.Equal(t, int64(1), hits[0].SeedID)
	require.Equal(t, "MISLEADING.CLAIM", hits[0].Issue)
	require.Greater(t, hits[0].Score, 0.9)

	require.NoError(t, index.Delete(ctx, []int64{1}))
	hits, err = index.Search(ctx, normalize([]float32{1, 0, 0, 0}), "US", 5)
	require.NoError(t, err)
	for _, hit := range hits {
		require.NotEqual(t, int64(1), hit.SeedID)
	}
}
