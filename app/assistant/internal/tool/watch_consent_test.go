package tool

import (
	"context"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/watch"
	"esx/pkg/errx"
	"strconv"
	"testing"
)

func TestWatchToolsRecheckCurrentConsent(t *testing.T) {
	for _, version := range []int32{0, 1, 2, 3} {
		for _, name := range []string{CreateWatchTask, UpdateWatchTask, DeleteWatchTask} {
			t.Run(strconv.Itoa(int(version))+"/"+name, func(t *testing.T) {
				ctx := context.Background()
				st := store.NewMemoryStore()
				w := watch.NewMapStore()
				_, err := w.Create(ctx, watch.Task{UserID: 1, ConditionType: watch.KeywordNewPost, TargetType: "keyword", TargetText: "old"})
				if err != nil {
					t.Fatal(err)
				}
				registry, err := NewRegistry(Clients{Watch: w, Store: st}, []string{name})
				if err != nil {
					t.Fatal(err)
				}
				// Frozen run version remains 2; only authoritative current consent changes.
				st.SetAgentConsent(1, version)
				args := `{"id":1,"expected_version":1}`
				if name == CreateWatchTask {
					args = `{"condition_type":"keyword_new_post","target_type":"keyword","target_text":"new"}`
				}
				if name == UpdateWatchTask {
					args = `{"id":1,"enabled":false,"expected_version":1}`
				}
				_, _, err = registry.Call(ctx, &Session{UserID: 1, Source: store.SourceUser, ConsentVersion: CurrentConsentVersion}, name, "call", args)
				if version == CurrentConsentVersion {
					if err != nil {
						t.Fatal(err)
					}
				} else {
					if !errx.Is(err, errx.AgentNotAuthorized) {
						t.Fatalf("error=%v", err)
					}
					tasks, _ := w.ListTasks(ctx, 1)
					if len(tasks) != 1 || tasks[0].Version != 1 || !tasks[0].Enabled {
						t.Fatalf("denied mutation changed tasks %+v", tasks)
					}
				}
			})
		}
	}
}
