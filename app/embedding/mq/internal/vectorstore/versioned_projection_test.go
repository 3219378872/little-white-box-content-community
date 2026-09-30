package vectorstore

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"

	"esx/pkg/vectorprojection"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"github.com/stretchr/testify/require"
)

type storedProjection struct {
	id, revision int64
	deleted      bool
	vector       []float32
}
type durableMilvus struct {
	client.Client
	mu      sync.Mutex
	rows    map[string]storedProjection
	delayed chan struct{}
	release chan struct{}
	loseACK bool
}

func (f *durableMilvus) Upsert(_ context.Context, _, _ string, columns ...entity.Column) (entity.Column, error) {
	byName := client.ResultSet(columns)
	ids := byName.GetColumn("post_id").(*entity.ColumnInt64).Data()
	revisions := byName.GetColumn("revision").(*entity.ColumnInt64).Data()
	keys := byName.GetColumn("projection_id").(*entity.ColumnVarChar).Data()
	deleted := byName.GetColumn("deleted").(*entity.ColumnBool).Data()
	vectors := byName.GetColumn("embedding").(*entity.ColumnFloatVector).Data()
	if f.delayed != nil && revisions[0] == 2 {
		close(f.delayed)
		<-f.release
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, id := range ids {
		f.rows[keys[i]] = storedProjection{id: id, revision: revisions[i], deleted: deleted[i], vector: append([]float32(nil), vectors[i]...)}
	}
	if f.loseACK {
		f.loseACK = false
		return nil, errors.New("durable remote write with lost ACK")
	}
	return nil, nil
}
func (f *durableMilvus) Query(_ context.Context, _ string, _ []string, expr string, _ []string, opts ...client.SearchQueryOptionFunc) (client.ResultSet, error) {
	options := &client.SearchQueryOption{}
	for _, opt := range opts {
		opt(options)
	}
	if options.ConsistencyLevel != entity.ClStrong {
		return nil, errors.New("projection read must be strongly consistent")
	}
	start, end := strings.Index(expr, "["), strings.Index(expr, "]")
	wanted := map[int64]bool{}
	for _, part := range strings.Split(expr[start+1:end], ",") {
		id, _ := strconv.ParseInt(part, 10, 64)
		wanted[id] = true
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids, revisions []int64
	var deleted []bool
	var vectors [][]float32
	for _, row := range f.rows {
		if wanted[row.id] {
			ids = append(ids, row.id)
			revisions = append(revisions, row.revision)
			deleted = append(deleted, row.deleted)
			vectors = append(vectors, row.vector)
		}
	}
	return client.ResultSet{entity.NewColumnInt64("post_id", ids), entity.NewColumnInt64("revision", revisions), entity.NewColumnBool("deleted", deleted), entity.NewColumnFloatVector("embedding", 2, vectors)}, nil
}
func projectionRecord(revision int64) Record {
	return Record{PostID: 99, Revision: revision, Vector: []float32{float32(revision), 1}, ModelVersion: "test", Dimension: 2}
}
func TestMilvusAdapterDurableVersionsAndTombstone(t *testing.T) {
	ctx := context.Background()
	remote := &durableMilvus{rows: map[string]storedProjection{}}
	store := &MilvusVectorStore{cli: remote, collection: "test", dim: 2}
	require.NoError(t, store.Upsert(ctx, projectionRecord(3)))
	require.NoError(t, store.Upsert(ctx, projectionRecord(2)))
	rev, err := store.CurrentRevision(ctx, 99)
	require.NoError(t, err)
	require.Equal(t, int64(3), rev)
	remote.loseACK = true
	require.Error(t, store.Delete(ctx, 99, 4))
	// A different adapter instance after restart shares only durable remote data.
	store = &MilvusVectorStore{cli: remote, collection: "test", dim: 2}
	require.NoError(t, store.Upsert(ctx, projectionRecord(3)))
	require.NoError(t, store.Delete(ctx, 99, 4))
	require.NoError(t, store.Upsert(ctx, projectionRecord(4)))
	rows, err := vectorprojection.Latest(ctx, remote, "test", []int64{99}, true)
	require.NoError(t, err)
	require.Equal(t, int64(4), rows[99].Revision)
	require.True(t, rows[99].Deleted)
	require.Len(t, remote.rows, 4, "same-revision retries must use stable keys")
}
func TestMilvusDelayedOldWriteCannotReplaceNewTombstone(t *testing.T) {
	remote := &durableMilvus{rows: map[string]storedProjection{}, delayed: make(chan struct{}), release: make(chan struct{})}
	store := &MilvusVectorStore{cli: remote, collection: "test", dim: 2}
	done := make(chan error, 1)
	go func() { done <- store.Upsert(context.Background(), projectionRecord(2)) }()
	<-remote.delayed
	require.NoError(t, store.Delete(context.Background(), 99, 3))
	close(remote.release)
	require.NoError(t, <-done)
	rows, err := vectorprojection.Latest(context.Background(), remote, "test", []int64{99}, false)
	require.NoError(t, err)
	require.Equal(t, int64(3), rows[99].Revision)
	require.True(t, rows[99].Deleted)
}

type aliasPinClient struct {
	client.Client
	schema *entity.Schema
	writes []string
}

func (c *aliasPinClient) HasCollection(context.Context, string) (bool, error) { return true, nil }
func (c *aliasPinClient) DescribeCollection(_ context.Context, name string) (*entity.Collection, error) {
	return &entity.Collection{Name: name, Schema: c.schema}, nil
}
func (c *aliasPinClient) LoadCollection(context.Context, string, bool, ...client.LoadCollectionOption) error {
	return nil
}
func (c *aliasPinClient) Upsert(_ context.Context, name, _ string, _ ...entity.Column) (entity.Column, error) {
	c.writes = append(c.writes, name)
	return nil, nil
}
func TestRuntimeWritesRemainPinnedAcrossAliasPromotion(t *testing.T) {
	schema := (&MilvusVectorStore{collection: "old_physical", dim: 2}).schema()
	remote := &aliasPinClient{schema: schema}
	writer := &MilvusVectorStore{cli: remote, collection: "active_alias", dim: 2}
	require.NoError(t, writer.OpenCollection(context.Background()))
	require.Equal(t, "old_physical", writer.collection)
	remote.schema = (&MilvusVectorStore{collection: "new_physical", dim: 2}).schema() // alias promotion
	require.NoError(t, writer.Upsert(context.Background(), projectionRecord(2)))
	require.NoError(t, writer.Delete(context.Background(), 99, 3))
	require.Equal(t, []string{"old_physical", "old_physical"}, remote.writes)
	restarted := &MilvusVectorStore{cli: remote, collection: "active_alias", dim: 2}
	require.NoError(t, restarted.OpenCollection(context.Background()))
	require.Equal(t, "new_physical", restarted.collection)
}
