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

type capturedUpdate struct {
	method, path, version, retryOnConflict string
	body                                   map[string]any
}

func newUpdateServer(t *testing.T, status int, got *capturedUpdate) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		got.method, got.path = r.Method, r.URL.Path
		got.version = r.URL.Query().Get("version")
		got.retryOnConflict = r.URL.Query().Get("retry_on_conflict")
		if err := json.NewDecoder(r.Body).Decode(&got.body); err != nil {
			t.Errorf("decode update body: %v", err)
		}
		if status != http.StatusOK {
			http.Error(w, `{"error":{"type":"version_conflict_engine_exception"}}`, status)
			return
		}
		_, _ = w.Write([]byte(`{"result":"updated"}`))
	}))
}

func scriptParams(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	script, _ := body["script"].(map[string]any)
	params, _ := script["params"].(map[string]any)
	if params == nil {
		t.Fatalf("missing script params: %v", body)
	}
	return params
}

func TestIndexGuardsByRevisionInsteadOfExternalVersion(t *testing.T) {
	var got capturedUpdate
	server := newUpdateServer(t, http.StatusOK, &got)
	defer server.Close()

	idx, err := NewESIndexer([]string{server.URL}, "xbh_posts")
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Index(t.Context(), IndexDoc{
		DocID: "9", Revision: 2, Body: map[string]any{"title": "B"},
	}); err != nil {
		t.Fatal(err)
	}
	if got.method != http.MethodPost || got.path != "/xbh_posts/_update/9" {
		t.Fatalf("request=%s %s", got.method, got.path)
	}
	if got.version != "" || got.retryOnConflict != "3" {
		t.Fatalf("version=%q retry_on_conflict=%q", got.version, got.retryOnConflict)
	}
	if got.body["scripted_upsert"] != true {
		t.Fatalf("index must be a scripted upsert: %v", got.body)
	}
	params := scriptParams(t, got.body)
	doc, _ := params["doc"].(map[string]any)
	if params["revision"] != float64(2) || doc["revision"] != float64(2) || doc["title"] != "B" {
		t.Fatalf("params=%v", params)
	}
}

func TestIndexReturnsConflictForMessageRetry(t *testing.T) {
	var got capturedUpdate
	server := newUpdateServer(t, http.StatusConflict, &got)
	defer server.Close()

	idx, err := NewESIndexer([]string{server.URL}, "xbh_posts")
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Index(t.Context(), IndexDoc{DocID: "9", Revision: 2, Body: map[string]any{}}); err == nil {
		t.Fatal("a conflict left after retry_on_conflict must be retried, not acknowledged")
	}
}

func TestDeleteWritesRevisionTombstone(t *testing.T) {
	var got capturedUpdate
	server := newUpdateServer(t, http.StatusOK, &got)
	defer server.Close()

	idx, err := NewESIndexer([]string{server.URL}, "xbh_posts")
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Delete(t.Context(), "9", 3); err != nil {
		t.Fatal(err)
	}
	if got.method != http.MethodPost || got.path != "/xbh_posts/_update/9" || got.retryOnConflict != "3" {
		t.Fatalf("request=%s %s retry_on_conflict=%q", got.method, got.path, got.retryOnConflict)
	}
	if got.body["scripted_upsert"] != true || scriptParams(t, got.body)["revision"] != float64(3) {
		t.Fatalf("body=%v", got.body)
	}
}

func TestDeleteReturnsConflictForMessageRetry(t *testing.T) {
	var got capturedUpdate
	server := newUpdateServer(t, http.StatusConflict, &got)
	defer server.Close()

	idx, err := NewESIndexer([]string{server.URL}, "xbh_posts")
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Delete(t.Context(), "9", 2); err == nil {
		t.Fatal("a conflict left after retry_on_conflict must be retried, not acknowledged")
	}
}
