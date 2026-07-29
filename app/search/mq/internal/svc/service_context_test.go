package svc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"esx/app/search/mq/internal/config"
)

func TestBuildIndexerRejectsMissingConfiguration(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.ESConfig
		want string
	}{
		{name: "address", cfg: config.ESConfig{Index: "xbh_posts"}, want: "address"},
		{name: "index", cfg: config.ESConfig{Addresses: []string{"http://localhost:9200"}}, want: "index"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildIndexer(tt.cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err=%v want substring %q", err, tt.want)
			}
		})
	}
}

func TestBuildIndexerDoesNotSilentlyDegradeWhenESFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	searchIndexer, err := buildIndexer(config.ESConfig{
		Addresses: []string{server.URL}, Index: "xbh_posts",
	})
	if err == nil || searchIndexer != nil || !strings.Contains(err.Error(), "ensure ES index") {
		t.Fatalf("indexer=%T err=%v", searchIndexer, err)
	}
}
