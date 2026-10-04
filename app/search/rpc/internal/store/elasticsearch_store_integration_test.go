//go:build integration

package store

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"esx/pkg/testutil"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tags field mirrors the production post mapping: a plain keyword field.
const tagProbeMapping = `{"mappings":{"properties":{
  "post_id":{"type":"long"},
  "title":{"type":"text","analyzer":"cjk"},
  "body":{"type":"text","analyzer":"cjk"},
  "tags":{"type":"keyword"}
}}}`

func TestSearchTagsKeywordAggregationOnElasticsearch(t *testing.T) {
	env := testutil.SetupElasticsearchEnv(t)
	defer env.Close()
	ctx := context.Background()
	const index = "xbh_posts_tag_probe"
	searchStore, err := NewElasticsearchStore([]string{env.URL}, index,
		WithBasicAuth(env.Username, env.Password),
		func(c *elasticsearch.Config) { c.CACert = env.CACert })
	require.NoError(t, err)

	res, err := esapi.IndicesCreateRequest{Index: index, Body: strings.NewReader(tagProbeMapping)}.Do(ctx, searchStore.client)
	require.NoError(t, err)
	require.False(t, res.IsError(), res.String())
	_ = res.Body.Close()

	var bulk strings.Builder
	id := 0
	add := func(count int, tags ...string) {
		for range count {
			id++
			fmt.Fprintf(&bulk, "{\"index\":{\"_id\":\"%d\"}}\n", id)
			quoted := make([]string, len(tags))
			for i, tag := range tags {
				quoted[i] = fmt.Sprintf("%q", tag)
			}
			fmt.Fprintf(&bulk, "{\"post_id\":%d,\"title\":\"t\",\"body\":\"b\",\"tags\":[%s]}\n", id, strings.Join(quoted, ","))
		}
	}
	// 30 popular tags outrank the long-tail match; with limit 1 the old
	// top-(limit×20) candidate window could never reach "Go-Rare".
	for i := range 30 {
		add(5, fmt.Sprintf("popular-%02d", i))
	}
	add(3, "Golang")
	add(1, "Go-Rare")
	add(2, "摄影", "手机摄影")
	add(1, "c++")
	res, err = esapi.BulkRequest{Index: index, Body: strings.NewReader(bulk.String()), Refresh: "true"}.Do(ctx, searchStore.client)
	require.NoError(t, err)
	require.False(t, res.IsError(), res.String())
	_ = res.Body.Close()

	tags, err := searchStore.SearchTags(ctx, "gO", 5)
	require.NoError(t, err)
	assert.Equal(t, []Tag{{Name: "Golang", PostCount: 3}, {Name: "Go-Rare", PostCount: 1}}, tags,
		"case-insensitive match; doc_count is the tag's post count")

	tags, err = searchStore.SearchTags(ctx, "rare", 1)
	require.NoError(t, err)
	assert.Equal(t, []Tag{{Name: "Go-Rare", PostCount: 1}}, tags, "long-tail tags are recalled")

	tags, err = searchStore.SearchTags(ctx, "摄影", 5)
	require.NoError(t, err)
	assert.ElementsMatch(t, []Tag{{Name: "摄影", PostCount: 2}, {Name: "手机摄影", PostCount: 2}}, tags)

	tags, err = searchStore.SearchTags(ctx, "C++", 5)
	require.NoError(t, err)
	assert.Equal(t, []Tag{{Name: "c++", PostCount: 1}}, tags, "regex metacharacters match literally")

	tags, err = searchStore.SearchTags(ctx, "absent", 5)
	require.NoError(t, err)
	assert.Empty(t, tags)
}
