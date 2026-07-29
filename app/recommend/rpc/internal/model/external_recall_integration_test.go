//go:build integration

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"esx/pkg/testutil"

	milvusclient "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestElasticsearchRecallAgainstRealIndex(t *testing.T) {
	env := testutil.SetupElasticsearchEnv(t)
	t.Cleanup(env.Close)

	const index = "recommend_external_recall_test"
	documents := []struct {
		id    int64
		title string
		body  string
	}{
		{id: 41001, title: "vector retrieval recommendation engine", body: "milvus semantic vector nearest neighbor content recommendation"},
		{id: 41002, title: "vector search recommendation ranking", body: "semantic vector retrieval for content recommendation with nearest neighbors"},
		{id: 41003, title: "garden watering schedule", body: "flowers soil sunlight and seasonal gardening notes"},
	}
	for _, document := range documents {
		indexElasticsearchDocument(t, env.URL, index, document.id, map[string]any{
			"post_id": document.id,
			"title":   document.title,
			"body":    document.body,
		})
	}

	source, err := NewElasticsearchPostRecallSource(
		[]string{env.URL}, index, env.Username, env.Password, "v2", nil, 10*time.Second,
	)
	require.NoError(t, err)
	candidates, err := source.Recall(context.Background(), RecallRequest{SeedPostID: 41001, Limit: 3})
	require.NoError(t, err)
	require.NotEmpty(t, candidates)
	assert.Equal(t, int64(41002), candidates[0].PostID)
	for _, candidate := range candidates {
		assert.NotEqual(t, int64(41001), candidate.PostID)
		assert.Equal(t, "es", candidate.RecallSource)
	}
}

func TestMilvusRecallAgainstRealCollection(t *testing.T) {
	env := testutil.SetupMilvusEnv(t)
	t.Cleanup(env.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := connectMilvusForRecallTest(t, ctx, env.Address)
	t.Cleanup(func() { _ = client.Close() })

	const (
		collection = "recommend_external_recall_test"
		dimension  = 4
	)
	schema := &entity.Schema{
		CollectionName: collection,
		AutoID:         false,
		Fields: []*entity.Field{
			{Name: "post_id", DataType: entity.FieldTypeInt64, PrimaryKey: true, AutoID: false},
			{Name: "embedding", DataType: entity.FieldTypeFloatVector, TypeParams: map[string]string{"dim": strconv.Itoa(dimension)}},
		},
	}
	require.NoError(t, client.CreateCollection(ctx, schema, 1))
	index, err := entity.NewIndexIvfFlat(entity.L2, 1)
	require.NoError(t, err)
	require.NoError(t, client.CreateIndex(ctx, collection, "embedding", index, false))
	_, err = client.Insert(ctx, collection, "",
		entity.NewColumnInt64("post_id", []int64{42001, 42002, 42003}),
		entity.NewColumnFloatVector("embedding", dimension, [][]float32{
			{1, 0, 0, 0},
			{0.9, 0.1, 0, 0},
			{-1, 0, 0, 0},
		}),
	)
	require.NoError(t, err)
	require.NoError(t, client.Flush(ctx, collection, false))
	require.NoError(t, client.LoadCollection(ctx, collection, false))

	source := NewMilvusPostRecallSource(
		env.Address, collection, "", "", "", "v2", 1, nil, 20*time.Second,
	)
	t.Cleanup(func() { _ = source.Close() })
	candidates, err := source.Recall(ctx, RecallRequest{SeedPostID: 42001, Limit: 2})
	require.NoError(t, err)
	require.NotEmpty(t, candidates)
	assert.Equal(t, int64(42002), candidates[0].PostID)
	for _, candidate := range candidates {
		assert.NotEqual(t, int64(42001), candidate.PostID)
		assert.Equal(t, "milvus", candidate.RecallSource)
	}
}

func indexElasticsearchDocument(t *testing.T, endpoint, index string, id int64, document map[string]any) {
	t.Helper()
	body, err := json.Marshal(document)
	require.NoError(t, err)
	requestURL := fmt.Sprintf("%s/%s/_doc/%d?refresh=wait_for", endpoint, url.PathEscape(index), id)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPut, requestURL, bytes.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	response, err := (&http.Client{Transport: transport}).Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		require.Failf(t, "index Elasticsearch document", "status=%s body=%s", response.Status, raw)
	}
}

func connectMilvusForRecallTest(t *testing.T, ctx context.Context, address string) milvusclient.Client {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		client, err := milvusclient.NewClient(connectCtx, milvusclient.Config{
			Address: address,
			DialOptions: []grpc.DialOption{
				grpc.WithNoProxy(),
			},
		})
		cancel()
		if err == nil {
			return client
		}
		lastErr = err
		select {
		case <-ctx.Done():
			require.NoError(t, ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
	require.NoError(t, lastErr)
	return nil
}
