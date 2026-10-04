//go:build integration

package indexer

import (
	"bytes"
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
		PostID: 10003, AuthorID: 42, Title: "to-be-deleted", Revision: 1,
	}
	require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(e)))
	require.NoError(t, esIdx.Delete(ctx, "10003", 2))
	require.NoError(t, esIdx.Refresh(ctx))

	source := getSource(t, "10003")
	assert.Nil(t, source["title"], "deleted doc must lose its searchable fields")
	assert.Equal(t, true, source["deleted"])
	assert.Equal(t, float64(2), source["revision"])
	assert.Zero(t, countMatches(t, "to-be-deleted"), "tombstone must not be searchable")
}

// 计数补丁走 _update 会推高 ES _version；之后的编辑仍须按 revision 生效。
func TestESIndexer_UpdateAfterCountPatch_Applies(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	created := event.PostEvent{
		EventID: 11, EventTime: now, Type: event.PostEventCreated,
		PostID: 10011, AuthorID: 42, Title: "before edit", Revision: 1, StatsSeq: now,
	}
	require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(created)))
	counted := event.PostEvent{
		EventID: 12, EventTime: now + 1, Type: event.PostEventCounted,
		PostID: 10011, LikeCount: 1, StatsSeq: now + 1,
	}
	require.NoError(t, esIdx.PatchCounts(ctx, PostEventToIndexDoc(counted)))
	require.NoError(t, esIdx.PatchCounts(ctx, PostEventToIndexDoc(counted)))

	updated := created
	updated.EventID, updated.Type, updated.Title, updated.Revision = 13, event.PostEventUpdated, "after edit", 2
	require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(updated)))
	require.NoError(t, esIdx.Refresh(ctx))

	source := getSource(t, "10011")
	assert.Equal(t, "after edit", source["title"])
	assert.Equal(t, float64(2), source["revision"])
}

func TestESIndexer_DeleteAfterCountPatch_Tombstones(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	created := event.PostEvent{
		EventID: 21, EventTime: now, Type: event.PostEventCreated,
		PostID: 10021, AuthorID: 42, Title: "liked then deleted", Revision: 1, StatsSeq: now,
	}
	require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(created)))
	counted := event.PostEvent{
		EventID: 22, EventTime: now + 1, Type: event.PostEventCounted,
		PostID: 10021, LikeCount: 1, StatsSeq: now + 1,
	}
	require.NoError(t, esIdx.PatchCounts(ctx, PostEventToIndexDoc(counted)))
	require.NoError(t, esIdx.Delete(ctx, "10021", 2))

	// 删除后再到达的计数补丁不能复活文档。
	counted.EventID, counted.LikeCount, counted.StatsSeq = 23, 2, now+2
	require.NoError(t, esIdx.PatchCounts(ctx, PostEventToIndexDoc(counted)))
	require.NoError(t, esIdx.Refresh(ctx))

	source := getSource(t, "10021")
	assert.Equal(t, map[string]any{"revision": float64(2), "deleted": true}, source)
}

func TestESIndexer_StaleWritesAfterDelete_AreIgnored(t *testing.T) {
	ctx := context.Background()
	created := event.PostEvent{
		EventID: 31, EventTime: time.Now().UnixMilli(), Type: event.PostEventCreated,
		PostID: 10031, AuthorID: 42, Title: "late retry", Revision: 1,
	}
	require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(created)))
	require.NoError(t, esIdx.Delete(ctx, "10031", 2))

	// 失败重试的 create 晚于删除到达，同 revision 的删除重投也到达。
	require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(created)))
	require.NoError(t, esIdx.Delete(ctx, "10031", 2))
	require.NoError(t, esIdx.Refresh(ctx))

	assert.Equal(t, map[string]any{"revision": float64(2), "deleted": true}, getSource(t, "10031"))

	// 更高 revision 的重新发布覆盖墓碑。
	republished := created
	republished.EventID, republished.Type, republished.Title, republished.Revision = 32, event.PostEventUpdated, "republished", 3
	require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(republished)))
	require.NoError(t, esIdx.Refresh(ctx))
	assert.Equal(t, "republished", getSource(t, "10031")["title"])
}

func TestESIndexer_RedeliveredRevision_IsNoop(t *testing.T) {
	ctx := context.Background()
	first := event.PostEvent{
		EventID: 41, EventTime: time.Now().UnixMilli(), Type: event.PostEventUpdated,
		PostID: 10041, AuthorID: 42, Title: "revision two", Revision: 2,
	}
	require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(first)))
	older := first
	older.EventID, older.Type, older.Title, older.Revision = 40, event.PostEventCreated, "revision one", 1
	same := first
	same.Title = "revision two replay"
	require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(older)))
	require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(same)))
	require.NoError(t, esIdx.Refresh(ctx))

	assert.Equal(t, "revision two", getSource(t, "10041")["title"])
}

// 重建写入的 Body 不带 revision 字段；索引仍须记录 revision 以挡住更旧的消息。
func TestESIndexer_RevisionStoredWhenBodyOmitsIt(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, esIdx.Index(ctx, IndexDoc{
		DocID: "10051", Type: "rebuild", Revision: 3,
		Body: map[string]any{"post_id": 10051, "author_id": 42, "title": "rebuilt"},
	}))
	stale := event.PostEvent{
		EventID: 51, EventTime: time.Now().UnixMilli(), Type: event.PostEventUpdated,
		PostID: 10051, AuthorID: 42, Title: "stale", Revision: 2,
	}
	require.NoError(t, esIdx.Index(ctx, PostEventToIndexDoc(stale)))
	require.NoError(t, esIdx.Refresh(ctx))

	source := getSource(t, "10051")
	assert.Equal(t, "rebuilt", source["title"])
	assert.Equal(t, float64(3), source["revision"])
}

func getSource(t *testing.T, docID string) map[string]any {
	t.Helper()
	res, err := esapi.GetRequest{Index: indexN, DocumentID: docID}.Do(context.Background(), esIdx.client)
	require.NoError(t, err)
	defer res.Body.Close()
	require.False(t, res.IsError(), "get %s status=%s", docID, res.Status())
	var got struct {
		Source map[string]any `json:"_source"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	return got.Source
}

func countMatches(t *testing.T, text string) int {
	t.Helper()
	body, err := json.Marshal(map[string]any{"query": map[string]any{"multi_match": map[string]any{
		"query": text, "fields": []string{"title", "body"},
	}}})
	require.NoError(t, err)
	res, err := esapi.CountRequest{Index: []string{indexN}, Body: bytes.NewReader(body)}.Do(context.Background(), esIdx.client)
	require.NoError(t, err)
	defer res.Body.Close()
	require.False(t, res.IsError(), "count status=%s", res.Status())
	var got struct {
		Count int `json:"count"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	return got.Count
}

func TestESIndexer_Delete_MissingDoc_NoError(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, esIdx.Delete(ctx, "99999999", 1))
}

func TestESIndexer_EnsureIndex_Idempotent(t *testing.T) {
	require.NoError(t, esIdx.EnsureIndex(context.Background()))
	require.NoError(t, esIdx.EnsureIndex(context.Background()))
}
