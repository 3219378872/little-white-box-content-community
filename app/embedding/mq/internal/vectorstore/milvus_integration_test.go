//go:build integration

package vectorstore

import (
	"context"
	"os"
	"testing"
	"time"

	"esx/pkg/testutil"
	"esx/pkg/vectorprojection"

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
	rows, err := vectorprojection.Latest(ctx, store.cli, testColl, ids, false)
	require.NoError(t, err)
	result := make([]int64, 0)
	for _, id := range ids {
		if row, ok := rows[id]; ok && !row.Deleted {
			result = append(result, id)
		}
	}
	return result
}

func record(postID int64, seed float32) Record {
	return Record{
		PostID: postID, Revision: 1, Vector: sampleVec(seed), ModelVersion: "integration-model@v1", Dimension: testDim,
	}
}

func TestMilvus_Upsert_InsertsRecordWithMetadata(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, store.Upsert(ctx, record(30001, 0.1)))

	got := queryPostIDs(t, ctx, []int64{30001})
	assert.Equal(t, []int64{30001}, got)
	cols, err := store.cli.QueryByPks(ctx, testColl, []string{},
		entity.NewColumnVarChar("projection_id", []string{"30001:1"}), []string{"model_version", "dimension"})
	require.NoError(t, err)
	columnsByName := make(map[string]entity.Column, len(cols))
	for _, column := range cols {
		columnsByName[column.Name()] = column
	}
	modelVersions, ok := columnsByName["model_version"].(*entity.ColumnVarChar)
	require.True(t, ok)
	dimensions, ok := columnsByName["dimension"].(*entity.ColumnInt32)
	require.True(t, ok)
	assert.Equal(t, []string{"integration-model@v1"}, modelVersions.Data())
	assert.Equal(t, []int32{int32(testDim)}, dimensions.Data())
}

func TestMilvus_Upsert_OverwritesExisting(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, store.Upsert(ctx, record(30002, 0.2)))
	require.NoError(t, store.Upsert(ctx, record(30002, 0.5)))

	got := queryPostIDs(t, ctx, []int64{30002})
	assert.Equal(t, []int64{30002}, got)
}

func TestMilvus_Delete_RemovesRecord(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, store.Upsert(ctx, record(30003, 0.3)))
	// 先 Flush 确保插入可见，然后再 Delete
	require.NoError(t, store.cli.Flush(ctx, testColl, false))
	require.NoError(t, store.Delete(ctx, 30003, 2))

	got := queryPostIDs(t, ctx, []int64{30003})
	assert.Empty(t, got)
}

func TestMilvus_Upsert_DimMismatch_Errors(t *testing.T) {
	ctx := context.Background()
	err := store.Upsert(ctx, Record{PostID: 30004, Revision: 1, Vector: []float32{0.1}, ModelVersion: "integration-model@v1", Dimension: testDim})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dim mismatch")
}

func TestMilvusConcurrentOldWriteAndTombstoneSurviveReconnect(t *testing.T) {
	ctx := context.Background()
	errs := make(chan error, 3)
	for _, revision := range []int64{1, 2} {
		go func(revision int64) { r := record(30100, 0.1); r.Revision = revision; errs <- store.Upsert(ctx, r) }(revision)
	}
	go func() { errs <- store.Delete(ctx, 30100, 3) }()
	for range 3 {
		require.NoError(t, <-errs)
	}
	reconnected, err := NewMilvusVectorStore(ctx, milvusEnv.Address, testColl, testDim)
	require.NoError(t, err)
	defer reconnected.Close()
	r := record(30100, 0.4)
	r.Revision = 2
	require.NoError(t, reconnected.Upsert(ctx, r))
	rows, err := vectorprojection.Latest(ctx, reconnected.cli, testColl, []int64{30100}, false)
	require.NoError(t, err)
	require.True(t, rows[30100].Deleted)
	require.Equal(t, int64(3), rows[30100].Revision)
}

func TestMilvusAliasPromotionCannotRetargetOldWriter(t *testing.T) {
	ctx := context.Background()
	const alias = "xbh_projection_pin_current"
	require.NoError(t, store.cli.CreateAlias(ctx, testColl, alias))
	oldWriter, err := NewMilvusVectorStore(ctx, milvusEnv.Address, alias, testDim)
	require.NoError(t, err)
	defer oldWriter.Close()
	require.NoError(t, oldWriter.OpenCollection(ctx))
	require.Equal(t, testColl, oldWriter.collection, "server schema must expose canonical physical name")
	newTarget, err := NewMilvusVectorStore(ctx, milvusEnv.Address, "xbh_projection_pin_new", testDim)
	require.NoError(t, err)
	defer newTarget.Close()
	require.NoError(t, newTarget.EnsureCollection(ctx))
	require.NoError(t, newTarget.PromoteAlias(ctx, alias))
	require.NoError(t, oldWriter.Upsert(ctx, record(30101, 0.1)))
	current, err := newTarget.CurrentRevision(ctx, 30101)
	require.NoError(t, err)
	require.Zero(t, current, "old writer must not enter newly promoted collection")
	restarted, err := NewMilvusVectorStore(ctx, milvusEnv.Address, alias, testDim)
	require.NoError(t, err)
	defer restarted.Close()
	require.NoError(t, restarted.OpenCollection(ctx))
	require.Equal(t, "xbh_projection_pin_new", restarted.collection)
}
