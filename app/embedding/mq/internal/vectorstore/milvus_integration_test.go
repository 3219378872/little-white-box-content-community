//go:build integration

package vectorstore

import (
	"context"
	"os"
	"testing"
	"time"

	"esx/pkg/testutil"

	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	milvusEnv *testutil.MilvusEnv
	store     *MilvusVectorStore
	testColl  = "xbh_post_embeddings_test"
	testDim   = 16 // 测试用小维度，加快 index 构建
)

func TestMain(m *testing.M) {
	milvusEnv = testutil.SetupMilvusEnvM()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	s, err := NewMilvusVectorStore(ctx, milvusEnv.Address, testColl, testDim)
	if err != nil {
		milvusEnv.Close()
		os.Stderr.WriteString("NewMilvusVectorStore: " + err.Error() + "\n")
		os.Exit(1)
	}
	if err := s.EnsureCollection(ctx); err != nil {
		milvusEnv.Close()
		os.Stderr.WriteString("EnsureCollection: " + err.Error() + "\n")
		os.Exit(1)
	}
	store = s

	code := m.Run()
	_ = store.Close()
	milvusEnv.Close()
	os.Exit(code)
}

func sampleVec(seed float32) []float32 {
	v := make([]float32, testDim)
	for i := range v {
		v[i] = seed + float32(i)*0.01
	}
	return v
}

// queryPostIDs flushes and queries by primary keys, returning the actual post_ids found.
// Milvus 在 growing segment 中的数据需要 Flush 才能被 query/search 看到。
func queryPostIDs(t *testing.T, ctx context.Context, ids []int64) []int64 {
	t.Helper()
	require.NoError(t, store.cli.Flush(ctx, testColl, false))
	cols, err := store.cli.QueryByPks(ctx, testColl, []string{},
		entity.NewColumnInt64("post_id", ids), []string{"post_id"})
	require.NoError(t, err)
	if len(cols) == 0 {
		return nil
	}
	idCol, ok := cols[0].(*entity.ColumnInt64)
	require.True(t, ok)
	return idCol.Data()
}

func TestMilvus_Upsert_InsertsRecord(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, store.Upsert(ctx, 30001, sampleVec(0.1)))

	got := queryPostIDs(t, ctx, []int64{30001})
	assert.Equal(t, []int64{30001}, got)
}

func TestMilvus_Upsert_OverwritesExisting(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, store.Upsert(ctx, 30002, sampleVec(0.2)))
	require.NoError(t, store.Upsert(ctx, 30002, sampleVec(0.5)))

	got := queryPostIDs(t, ctx, []int64{30002})
	assert.Equal(t, []int64{30002}, got)
}

func TestMilvus_Delete_RemovesRecord(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, store.Upsert(ctx, 30003, sampleVec(0.3)))
	// 先 Flush 确保插入可见，然后再 Delete
	require.NoError(t, store.cli.Flush(ctx, testColl, false))
	require.NoError(t, store.Delete(ctx, 30003))

	got := queryPostIDs(t, ctx, []int64{30003})
	assert.Empty(t, got)
}

func TestMilvus_Upsert_DimMismatch_Errors(t *testing.T) {
	ctx := context.Background()
	err := store.Upsert(ctx, 30004, []float32{0.1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dim mismatch")
}
