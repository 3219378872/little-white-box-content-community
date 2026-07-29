package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T, handler http.HandlerFunc) (*ElasticsearchStore, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		handler(w, r)
	}))
	searchStore, err := NewElasticsearchStore([]string{server.URL}, "xbh_posts")
	require.NoError(t, err)
	return searchStore, server
}

func TestHealthChecksPostIndex(t *testing.T) {
	searchStore, server := newTestStore(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodHead, r.Method)
		assert.Equal(t, "/xbh_posts", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()
	require.NoError(t, searchStore.Health(context.Background()))
}

func TestHealthFailsWhenPostIndexIsMissing(t *testing.T) {
	searchStore, server := newTestStore(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer server.Close()
	require.Error(t, searchStore.Health(context.Background()))
}

func TestSearchPostsBuildsQueryAndDecodesHighlights(t *testing.T) {
	searchStore, server := newTestStore(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/xbh_posts/_search", r.URL.Path)
		var query map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&query))
		assert.Equal(t, float64(20), query["from"])
		assert.Contains(t, fmt.Sprint(query["query"]), "golang")
		assert.Contains(t, fmt.Sprint(query["query"]), "backend")
		_, _ = w.Write([]byte(`{
			"hits":{"total":{"value":2,"relation":"eq"},"hits":[
				{"_source":{"post_id":7,"author_id":42,"title":"Go","body":"plain","created_at":123},"highlight":{"body":["<em>Go</em> is fast"]}}
			]}
		}`))
	})
	defer server.Close()

	result, err := searchStore.SearchPosts(context.Background(), PostQuery{
		Keyword: "golang", Page: 2, PageSize: 20, SortBy: 2, Tags: []string{"backend"},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Total)
	require.Len(t, result.Posts, 1)
	assert.Equal(t, int64(7), result.Posts[0].ID)
	assert.Equal(t, int64(42), result.Posts[0].AuthorID)
	assert.Equal(t, "<em>Go</em> is fast", result.Posts[0].ContentHighlight)
}

func TestPostIndexAggregationsDecodeTagsAndHotSearches(t *testing.T) {
	searchStore, server := newTestStore(t, func(w http.ResponseWriter, r *http.Request) {
		var query map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&query))
		_, _ = w.Write([]byte(`{"aggregations":{"tags":{"buckets":[{"key":"golang","doc_count":9},{"key":"database","doc_count":7},{"key":"gaming","doc_count":5}]}}}`))
	})
	defer server.Close()

	tags, err := searchStore.SearchTags(context.Background(), "GO", 2)
	require.NoError(t, err)
	assert.Equal(t, []Tag{{Name: "golang", PostCount: 9}}, tags)

	hot, err := searchStore.HotSearches(context.Background(), 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"golang", "database"}, hot)
}

func TestSearchPostsBuildsHotSort(t *testing.T) {
	searchStore, server := newTestStore(t, func(w http.ResponseWriter, r *http.Request) {
		var query map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&query))
		assert.Equal(t, []any{
			map[string]any{"like_count": map[string]any{"order": "desc"}},
			map[string]any{"comment_count": map[string]any{"order": "desc"}},
			map[string]any{"created_at": map[string]any{"order": "desc"}},
		}, query["sort"])
		_, _ = w.Write([]byte(`{"hits":{"total":{"value":0},"hits":[]}}`))
	})
	defer server.Close()

	_, err := searchStore.SearchPosts(context.Background(), PostQuery{
		Keyword: "go", Page: 1, PageSize: 20, SortBy: 3,
	})
	require.NoError(t, err)
}

func TestSearchFailureAndCancellation(t *testing.T) {
	t.Run("failure", func(t *testing.T) {
		searchStore, server := newTestStore(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"unavailable"}`))
		})
		defer server.Close()
		_, err := searchStore.SearchPosts(context.Background(), PostQuery{Keyword: "go", Page: 1, PageSize: 10})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "503")
	})

	t.Run("cancellation", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		searchStore, server := newTestStore(t, func(w http.ResponseWriter, r *http.Request) {
			close(started)
			<-release
			_, _ = w.Write([]byte(`{"hits":{"total":{"value":0},"hits":[]}}`))
		})
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := searchStore.SearchPosts(ctx, PostQuery{Keyword: "go", Page: 1, PageSize: 10})
			done <- err
		}()
		<-started
		cancel()
		var err error
		select {
		case err = <-done:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("search did not return after context cancellation")
		}
		close(release)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "context canceled"))
	})
}
