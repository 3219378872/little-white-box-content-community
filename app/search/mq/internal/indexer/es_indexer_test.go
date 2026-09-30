package indexer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPromoteToAliasMigratesLegacyPhysicalIndex(t *testing.T) {
	var actions []map[string]map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/_alias/xbh_posts":
			http.Error(w, `{"error":"alias missing"}`, http.StatusNotFound)
		case r.Method == http.MethodHead && r.URL.Path == "/xbh_posts":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/_aliases":
			var request struct {
				Actions []map[string]map[string]any `json:"actions"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode alias request: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			actions = request.Actions
			_, _ = w.Write([]byte(`{"acknowledged":true}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	target, err := NewESIndexer([]string{server.URL}, "xbh_posts_rebuild_1")
	if err != nil {
		t.Fatal(err)
	}
	if err := target.PromoteToAlias(t.Context(), "xbh_posts"); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 2 {
		t.Fatalf("actions=%v", actions)
	}
	if actions[0]["remove_index"]["index"] != "xbh_posts" {
		t.Fatalf("missing legacy index removal: %v", actions)
	}
	if actions[1]["add"]["index"] != "xbh_posts_rebuild_1" || actions[1]["add"]["alias"] != "xbh_posts" {
		t.Fatalf("unexpected alias addition: %v", actions)
	}
}

func TestPromoteToAliasReplacesExistingAlias(t *testing.T) {
	var actions []map[string]map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/_alias/xbh_posts":
			_, _ = w.Write([]byte(`{"xbh_posts_20260101":{"aliases":{"xbh_posts":{}}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/_aliases":
			var request struct {
				Actions []map[string]map[string]any `json:"actions"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode alias request: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			actions = request.Actions
			_, _ = w.Write([]byte(`{"acknowledged":true}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	target, err := NewESIndexer([]string{server.URL}, "xbh_posts_rebuild_2")
	if err != nil {
		t.Fatal(err)
	}
	if err := target.PromoteToAlias(t.Context(), "xbh_posts"); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 2 || actions[0]["remove"]["index"] != "xbh_posts_20260101" {
		t.Fatalf("unexpected actions: %v", actions)
	}
}

func TestPromoteToAliasRejectsInvalidAlias(t *testing.T) {
	target := &ESIndexer{index: "xbh_posts"}
	if err := target.PromoteToAlias(t.Context(), "xbh_posts"); err == nil {
		t.Fatal("expected identical index and alias to fail")
	}
}

func TestSnapshotConflictsRetryWithoutExternalVersionCoupling(t *testing.T) {
	for _, operation := range []string{"index", "delete"} {
		t.Run(operation, func(t *testing.T) {
			var version, versionType, retries, path string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				version = r.URL.Query().Get("version")
				versionType = r.URL.Query().Get("version_type")
				retries = r.URL.Query().Get("retry_on_conflict")
				path = r.URL.Path
				http.Error(w, `{"error":{"type":"version_conflict_engine_exception"}}`, http.StatusConflict)
			}))
			defer server.Close()
			idx, err := NewESIndexer([]string{server.URL}, "xbh_posts")
			if err != nil {
				t.Fatal(err)
			}
			if operation == "index" {
				err = idx.Index(t.Context(), IndexDoc{DocID: "9", Revision: 2, Body: map[string]any{"title": "B"}})
			} else {
				err = idx.Delete(t.Context(), "9", 2)
			}
			if err == nil {
				t.Fatal("exhausted update conflicts must retry; they are not evidence of a stale business revision")
			}
			if version != "" || versionType != "" || retries != "8" || path != "/xbh_posts/_update/9" {
				t.Fatalf("version=%s type=%s retries=%s path=%s", version, versionType, retries, path)
			}
		})
	}
}
