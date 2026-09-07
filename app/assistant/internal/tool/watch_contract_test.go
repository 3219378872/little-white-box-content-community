package tool

import (
	"context"
	"testing"

	"esx/app/assistant/internal/store"
	"esx/pkg/errx"
)

func TestWatchCapabilityBoundary(t *testing.T) {
	clients, _, _ := researchFixture(t)
	registry, err := NewRegistry(clients, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantWatch := map[string]bool{
		SearchPosts: true, SearchUsers: true, SearchTags: true,
		GetPost: true, GetPostComments: true, RecommendPosts: true,
		SimilarPosts: true, ComparePosts: true, GetMemory: true,
		SearchHistory: true, PresentSources: true, ReadSource: true, PublishAnswer: true,
	}
	for _, def := range registry.definitions {
		allowed := containsString(def.Metadata.Sources, store.SourceWatch)
		if allowed != wantWatch[def.Name] {
			t.Errorf("Watch authorization for %s: got %v", def.Name, allowed)
		}
		if allowed && (def.Metadata.Effect != EffectRead || def.Metadata.Confirmation) {
			t.Errorf("Watch gained a business write: %+v", def)
		}
	}

	for _, frozen := range []bool{false, true} {
		base := registry
		if frozen {
			base = registry.ResolveDefinitions(registry.Definitions())
		}
		for _, version := range []int{1, 2} {
			watch := ForClient(ForSource(base, store.SourceWatch, CurrentConsentVersion), version)
			for name, want := range map[string]bool{
				GetPost: true, PresentSources: version == 1,
				ReadSource: version == 2, PublishAnswer: version == 2, AskQuestions: false,
			} {
				if got := watch.Has(name); got != want {
					t.Errorf("frozen=%v protocol=%d tool=%s: got %v want %v", frozen, version, name, got, want)
				}
			}
		}
	}

	legacy := registry.ResolveDefinitions(ForClient(registry, 1).Definitions())
	watch := ForClient(ForSource(legacy, store.SourceWatch, CurrentConsentVersion), 2)
	if !watch.Has(PresentSources) || watch.Has(PublishAnswer) || watch.Has(ReadSource) {
		t.Fatal("Watch protocol rewrote a legacy frozen capability snapshot")
	}
	user := ForClient(ForSource(registry, store.SourceUser, CurrentConsentVersion), 2)
	if !user.Has(AskQuestions) || !user.Has(PublishAnswer) {
		t.Fatal("Watch restrictions removed user research tools")
	}
	review := ForClient(ForSource(registry, store.SourceMemoryReview, CurrentConsentVersion), 2)
	if review.Has(AskQuestions) || review.Has(ReadSource) || review.Has(PublishAnswer) {
		t.Fatal("research tools leaked to memory review")
	}
}

func TestWatchExecutionRejectsUnadvertisedCapabilities(t *testing.T) {
	clients, session, _ := researchFixture(t)
	session.Source = store.SourceWatch
	registry, err := NewRegistry(clients, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The unrestricted registry must reject forged calls independently of advertising.
	for _, name := range []string{
		AskQuestions, CreatePost, UpdatePost, DeletePost,
		AddMemory, ReplaceMemory, RemoveMemory, BatchMemory,
		CreateWatchTask, UpdateWatchTask, DeleteWatchTask, ListWatchTasks,
		GetMyFavorites, GetMyLikes, GetMyFollowing, GetMyPosts, WebSearch,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := registry.Prepare(context.Background(), session, name, `{}`); !errx.Is(err, errx.PermissionDenied) {
				t.Fatalf("prepare allowed %s: %v", name, err)
			}
			if _, _, err := registry.Call(context.Background(), session, name, "forged", `{}`); !errx.Is(err, errx.PermissionDenied) {
				t.Fatalf("execution allowed %s: %v", name, err)
			}
		})
	}
}
