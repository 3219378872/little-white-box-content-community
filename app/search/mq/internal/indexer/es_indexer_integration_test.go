//go:build integration

package indexer

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"testing"
	"time"

	"esx/pkg/event"
	"esx/pkg/testutil"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	esEnv  *testutil.ElasticsearchEnv
	esIdx  *ESIndexer
	indexN = "xbh_posts_test"
)

func TestMain(m *testing.M) {
	esEnv = testutil.SetupElasticsearchEnvM()

	idx, err := NewESIndexer(
		[]string{esEnv.URL}, indexN,
		WithBasicAuth(esEnv.Username, esEnv.Password),
		WithCACert(esEnv.CACert),
	)
	if err != nil {
		esEnv.Close()
		os.Stderr.WriteString("NewESIndexer: " + err.Error() + "\n")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := idx.EnsureIndex(ctx); err != nil {
		esEnv.Close()
		os.Stderr.WriteString("EnsureIndex: " + err.Error() + "\n")
		os.Exit(1)
	}
	esIdx = idx

	code := m.Run()
	esEnv.Close()
	os.Exit(code)
}

func TestESIndexer_IndexAndQuery(t *testing.T) {
	ctx := context.Background()
	e := event.PostEvent{
		EventID: 1, EventTime: time.Now().UnixMilli(), Type: event.PostEventCreated,
		PostID: 10001, AuthorID: 42, Title: "hello world",
		BodyExcerpt: "lorem ipsum dolor", Tags: []string{"tech"}, CategoryID: 3,
	}
	require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(e)))
	require.NoError(t, esIdx.Refresh(ctx))

	res, err := esapi.GetRequest{Index: indexN, DocumentID: strconv.FormatInt(e.PostID, 10)}.
		Do(ctx, esIdx.client)
	require.NoError(t, err)
	defer res.Body.Close()
	require.False(t, res.IsError(), "get should succeed, status=%s", res.Status())

	var got struct {
		Source map[string]any `json:"_source"`
	}
	raw, _ := io.ReadAll(res.Body)
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, "hello world", got.Source["title"])
	assert.Equal(t, float64(42), got.Source["author_id"])
}

func TestESIndexer_Upsert_OverwritesExistingDoc(t *testing.T) {
	ctx := context.Background()
	first := event.PostEvent{
		EventID: 2, EventTime: time.Now().UnixMilli(), Type: event.PostEventCreated,
		PostID: 10002, AuthorID: 42, Title: "first",
	}
	require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(first)))

	updated := first
	updated.EventID = 3
	updated.Type = event.PostEventUpdated
	updated.Title = "second"
	require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(updated)))
	require.NoError(t, esIdx.Refresh(ctx))

	res, err := esapi.GetRequest{Index: indexN, DocumentID: "10002"}.Do(ctx, esIdx.client)
	require.NoError(t, err)
	defer res.Body.Close()
	var got struct {
		Source map[string]any `json:"_source"`
	}
	raw, _ := io.ReadAll(res.Body)
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, "second", got.Source["title"])
}

func TestESIndexer_Delete_RemovesDoc(t *testing.T) {
	ctx := context.Background()
	e := event.PostEvent{
		EventID: 4, EventTime: time.Now().UnixMilli(), Type: event.PostEventCreated,
		PostID: 10003, AuthorID: 42, Title: "to-be-deleted",
	}
	require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(e)))
	require.NoError(t, esIdx.Delete(ctx, "10003"))
	require.NoError(t, esIdx.Refresh(ctx))

	res, err := esapi.GetRequest{Index: indexN, DocumentID: "10003"}.Do(ctx, esIdx.client)
	require.NoError(t, err)
	defer res.Body.Close()
	assert.True(t, res.IsError(), "deleted doc should not be found")
}

func TestESIndexer_Delete_MissingDoc_NoError(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, esIdx.Delete(ctx, "99999999"))
}

func TestESIndexer_EnsureIndex_Idempotent(t *testing.T) {
	require.NoError(t, esIdx.EnsureIndex(context.Background()))
	require.NoError(t, esIdx.EnsureIndex(context.Background()))
}
