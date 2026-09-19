package index

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"esx/app/assistant/internal/store"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/stretchr/testify/require"
)

func TestHistoryDeletionRetriesHTTPFailures(t *testing.T) {
	for _, status := range []int{200, 404, 403, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var responseStatus atomic.Int32
			responseStatus.Store(int32(status))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodDelete, r.Method)
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				w.WriteHeader(int(responseStatus.Load()))
			}))
			defer server.Close()
			es, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{server.URL}, DisableRetry: true})
			require.NoError(t, err)
			ctx := context.Background()
			mem := store.NewMemoryStore()
			require.NoError(t, mem.InsertOutbox(ctx, store.Outbox{UserID: 7, MessageID: 42, Op: store.IndexOpDelete}))
			client := &Client{es: es, store: mem}
			require.NoError(t, client.Relay(ctx))
			pending, err := mem.ListUnpublishedOutbox(ctx, 10)
			require.NoError(t, err)
			if status == 200 || status == 404 {
				require.Empty(t, pending)
				return
			}
			require.Len(t, pending, 1, "failed deletion must remain retryable")
			responseStatus.Store(200)
			require.NoError(t, client.Relay(ctx))
			pending, err = mem.ListUnpublishedOutbox(ctx, 10)
			require.NoError(t, err)
			require.Empty(t, pending, "recovery must finish deletion")
		})
	}
}
