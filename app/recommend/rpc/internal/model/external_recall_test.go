package model

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	milvusclient "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

func TestElasticsearchRecallQueriesMoreLikeThisAndExcludesSeed(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/posts/_search" || r.Method != http.MethodPost {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":{"hits":[{"_id":"10","_score":9,"_source":{"post_id":10}},{"_id":"11","_score":4.5,"_source":{"post_id":11}}]}}`))
	}))
	defer server.Close()

	source, err := NewElasticsearchPostRecallSource(
		[]string{server.URL}, "posts", "", "", "v2", nil, time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := source.Recall(context.Background(), RecallRequest{SeedPostID: 10, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].PostID != 11 || result[0].RecallSource != "es" || result[0].RecallScore != 4.5 {
		t.Fatalf("unexpected candidates: %#v", result)
	}
	encoded, _ := json.Marshal(body)
	if !strings.Contains(string(encoded), "more_like_this") || !strings.Contains(string(encoded), `"_id":"10"`) {
		t.Fatalf("query does not bind the seed document: %s", encoded)
	}
}

func TestElasticsearchRecallUsesRecentPositiveHistoryAndReportsFailures(t *testing.T) {
	redis := &fakeRedisClient{lists: map[string][]string{
		"feature:v2:u:42:recent": {
			`{"action":"exposure","target_id":1,"target_type":"post"}`,
			`{"action":"click","target_id":2,"target_type":"post"}`,
		},
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	source, err := NewElasticsearchPostRecallSource(
		[]string{server.URL}, "posts", "", "", "v2", redis, time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.Recall(context.Background(), RecallRequest{Identity: "u:42", Limit: 5})
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("error=%v want upstream status", err)
	}
}

type fakeMilvusRecallClient struct {
	queryResult  milvusclient.ResultSet
	searchResult []milvusclient.SearchResult
	closed       bool
}

func (f *fakeMilvusRecallClient) Query(
	context.Context, string, []string, string, []string, ...milvusclient.SearchQueryOptionFunc,
) (milvusclient.ResultSet, error) {
	return f.queryResult, nil
}

func (f *fakeMilvusRecallClient) Search(
	context.Context,
	string,
	[]string,
	string,
	[]string,
	[]entity.Vector,
	string,
	entity.MetricType,
	int,
	entity.SearchParam,
	...milvusclient.SearchQueryOptionFunc,
) ([]milvusclient.SearchResult, error) {
	return f.searchResult, nil
}

func (f *fakeMilvusRecallClient) Close() error {
	f.closed = true
	return nil
}

func TestMilvusRecallLoadsSeedVectorRanksDistanceAndCloses(t *testing.T) {
	fake := &fakeMilvusRecallClient{
		queryResult: milvusclient.ResultSet{
			entity.NewColumnFloatVector("embedding", 2, [][]float32{{1, 0}}),
		},
		searchResult: []milvusclient.SearchResult{{
			ResultCount: 3,
			IDs:         entity.NewColumnInt64("post_id", []int64{10, 12, 11}),
			Scores:      []float32{0, 0.5, 0.1},
		}},
	}
	source := NewMilvusPostRecallSource(
		"milvus:19530", "embeddings", "", "", "", "v2", 16, nil, time.Second,
	)
	source.factory = func(context.Context, milvusclient.Config) (milvusRecallClient, error) {
		return fake, nil
	}
	result, err := source.Recall(context.Background(), RecallRequest{SeedPostID: 10, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 || result[0].PostID != 11 || result[1].PostID != 12 || result[0].RecallSource != "milvus" {
		t.Fatalf("unexpected candidates: %#v", result)
	}
	if err := source.Close(); err != nil || !fake.closed {
		t.Fatalf("close err=%v closed=%v", err, fake.closed)
	}
	if _, err := source.Recall(context.Background(), RecallRequest{SeedPostID: 10, Limit: 5}); err == nil {
		t.Fatal("closed source accepted recall")
	}
}

func TestExternalRecallWithoutSeedIsNotApplicable(t *testing.T) {
	source, err := NewElasticsearchPostRecallSource(
		[]string{"http://127.0.0.1:9200"}, "posts", "", "", "v2", &fakeRedisClient{}, time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.Recall(context.Background(), RecallRequest{Identity: "u:42", Limit: 5})
	if !errors.Is(err, ErrNotApplicable) {
		t.Fatalf("error=%v want ErrNotApplicable", err)
	}
}
