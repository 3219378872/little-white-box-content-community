package indexer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"esx/pkg/event"

	"github.com/stretchr/testify/require"
)

// This captures real adapter requests and emulates their compare contract. It
// does NOT execute Painless; the corresponding ES integration test must do that.
func TestAdapterRequestsIndependentWatermarksAtSameWallTime(t *testing.T) {
	for _, order := range [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			var mu sync.Mutex
			state := map[string]any{}
			number := func(v any) int64 {
				if v == nil {
					return 0
				}
				n, err := v.(json.Number).Int64()
				require.NoError(t, err)
				return n
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				require.Empty(t, r.URL.Query().Get("version"))
				require.Empty(t, r.URL.Query().Get("version_type"))
				var request struct {
					Script struct {
						Source string         `json:"source"`
						Params map[string]any `json:"params"`
					} `json:"script"`
				}
				decoder := json.NewDecoder(r.Body)
				decoder.UseNumber()
				require.NoError(t, decoder.Decode(&request))
				mu.Lock()
				defer mu.Unlock()
				params := request.Script.Params
				switch request.Script.Source {
				case indexSnapshotScript:
					doc := params["doc"].(map[string]any)
					incoming := number(params["revision"])
					stored := number(state["revision"])
					if incoming > stored || (incoming <= 0 && stored <= 0 && state["projection_deleted"] != true) {
						for k, v := range doc {
							if k != "like_count" && k != "comment_count" && k != "stats_seq" {
								state[k] = v
							}
						}
						state["revision"] = params["revision"]
						state["projection_deleted"] = false
					}
					if state["post_id"] != nil && state["projection_deleted"] != true && (state["like_count"] == nil || number(doc["stats_seq"]) > number(state["stats_seq"])) {
						state["like_count"], state["comment_count"], state["stats_seq"] = doc["like_count"], doc["comment_count"], doc["stats_seq"]
					}
				case patchCountsScript:
					if state["post_id"] == nil {
						http.Error(w, `{"error":"not indexed"}`, 404)
						return
					}
					if number(params["stats_seq"]) > number(state["stats_seq"]) {
						state["like_count"], state["comment_count"], state["stats_seq"] = params["like_count"], params["comment_count"], params["stats_seq"]
					}
				default:
					t.Errorf("unexpected script")
				}
				fmt.Fprint(w, `{"result":"updated"}`)
			}))
			defer server.Close()
			idx, err := NewESIndexer([]string{server.URL}, "test")
			require.NoError(t, err)
			base := event.PostEvent{EventID: 1, EventTime: 100, PostID: 9, Type: event.PostEventCreated, Revision: 1, Title: "before", StatsSeq: 9000000000001}
			require.NoError(t, idx.Index(t.Context(), PostEventToIndexDoc(base)))
			events := []event.PostEvent{
				{EventID: 2, EventTime: 100, PostID: 9, Type: event.PostEventCounted, LikeCount: 1, StatsSeq: 9000000000002},
				{EventID: 3, EventTime: 100, PostID: 9, Type: event.PostEventUpdated, Revision: 2, Title: "edited", LikeCount: 1, StatsSeq: 9000000000003},
				{EventID: 4, EventTime: 100, PostID: 9, Type: event.PostEventCounted, LikeCount: 1, CommentCount: 1, StatsSeq: 9000000000004},
			}
			for _, i := range order {
				doc := PostEventToIndexDoc(events[i])
				if events[i].Type == event.PostEventCounted {
					require.NoError(t, idx.PatchCounts(t.Context(), doc))
				} else {
					require.NoError(t, idx.Index(t.Context(), doc))
				}
			}
			// A clock-skewed legacy event remains below the verified migration floor.
			require.NoError(t, idx.PatchCounts(t.Context(), PostEventToIndexDoc(event.PostEvent{EventID: 5, EventTime: 8000000000000, PostID: 9, Type: event.PostEventCounted, LikeCount: 99})))
			require.Equal(t, "edited", state["title"])
			require.Equal(t, int64(2), number(state["revision"]))
			require.Equal(t, int64(1), number(state["like_count"]))
			require.Equal(t, int64(1), number(state["comment_count"]))
			require.Equal(t, int64(9000000000004), number(state["stats_seq"]))
		})
	}
}
