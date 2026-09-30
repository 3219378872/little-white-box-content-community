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

func TestESIndexer_Delete_RemovesBodyAndRetainsTombstone(t *testing.T) {
	ctx := context.Background()
	e := event.PostEvent{
		EventID: 4, EventTime: time.Now().UnixMilli(), Type: event.PostEventCreated,
		PostID: 10003, AuthorID: 42, Title: "to-be-deleted", Revision: 1,
	}
	require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(e)))
	require.NoError(t, esIdx.Delete(ctx, "10003", 2))
	require.NoError(t, esIdx.Refresh(ctx))

	res, err := esapi.GetRequest{Index: indexN, DocumentID: "10003"}.Do(ctx, esIdx.client)
	require.NoError(t, err)
	defer res.Body.Close()
	require.False(t, res.IsError())
	var deleted struct {
		Source map[string]any `json:"_source"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&deleted))
	assert.Equal(t, true, deleted.Source["projection_deleted"])
	assert.NotContains(t, deleted.Source, "post_id")
	assert.NotContains(t, deleted.Source, "title")
}

func TestESIndexer_Delete_MissingDoc_NoError(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, esIdx.Delete(ctx, "99999999", 1))
}

func TestESIndexer_EnsureIndex_Idempotent(t *testing.T) {
	require.NoError(t, esIdx.EnsureIndex(context.Background()))
	require.NoError(t, esIdx.EnsureIndex(context.Background()))
}

func TestESIndependentBodyAndStatsOrderingAtIdenticalWallTime(t *testing.T) {
	for n, order := range [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			id := int64(10100 + n)
			ctx := context.Background()
			base := event.PostEvent{EventID: 1, EventTime: 100, PostID: id, Type: event.PostEventCreated, Revision: 1, Title: "before", StatsSeq: 9000000000001}
			require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(base)))
			events := []event.PostEvent{
				{EventID: 2, EventTime: 100, PostID: id, Type: event.PostEventCounted, LikeCount: 1, StatsSeq: 9000000000002},
				{EventID: 3, EventTime: 100, PostID: id, Type: event.PostEventUpdated, Revision: 2, Title: "edited", LikeCount: 1, StatsSeq: 9000000000003},
				{EventID: 4, EventTime: 100, PostID: id, Type: event.PostEventCounted, LikeCount: 1, CommentCount: 1, StatsSeq: 9000000000004},
			}
			for _, i := range order {
				doc := PostEventToIndexDoc(events[i])
				if events[i].Type == event.PostEventCounted {
					require.NoError(t, esIdx.PatchCounts(ctx, doc))
				} else {
					require.NoError(t, esIdx.Index(ctx, doc))
				}
			}
			source := readProjectionSource(t, id)
			require.Equal(t, "edited", source["title"])
			require.Equal(t, float64(1), source["like_count"])
			require.Equal(t, float64(1), source["comment_count"])
			require.Equal(t, float64(9000000000004), source["stats_seq"])
			require.NoError(t, esIdx.Delete(ctx, strconv.FormatInt(id, 10), 3))
			require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(events[1])))
			source = readProjectionSource(t, id)
			require.Equal(t, true, source["projection_deleted"])
			require.NotContains(t, source, "post_id")
		})
	}
}
func readProjectionSource(t *testing.T, id int64) map[string]any {
	t.Helper()
	res, err := (esapi.GetRequest{Index: indexN, DocumentID: strconv.FormatInt(id, 10)}).Do(context.Background(), esIdx.client)
	require.NoError(t, err)
	defer res.Body.Close()
	require.False(t, res.IsError())
	var payload struct {
		Source map[string]any `json:"_source"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	return payload.Source
}
