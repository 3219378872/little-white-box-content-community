package logic

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"

	"esx/app/content/rpc/contentservice"
	"esx/app/recommend/rpc/internal/config"
	"esx/app/recommend/rpc/internal/cursor"
	"esx/app/recommend/rpc/internal/model"
	"esx/app/recommend/rpc/internal/svc"
	"esx/app/recommend/rpc/xiaobaihe/recommend/pb"
	"esx/pkg/errx"
)

const testCursorSecret = "0123456789abcdef0123456789abcdef"

type fakePostSource struct {
	name   string
	recall func(context.Context, model.RecallRequest) ([]model.PostCandidate, error)
}

func (s fakePostSource) Name() string { return s.name }

func (s fakePostSource) Recall(ctx context.Context, req model.RecallRequest) ([]model.PostCandidate, error) {
	return s.recall(ctx, req)
}

type fakeUserSource struct {
	name   string
	recall func(context.Context, model.RecallRequest) ([]model.UserCandidate, error)
}

func (s fakeUserSource) Name() string { return s.name }

func (s fakeUserSource) Recall(ctx context.Context, req model.RecallRequest) ([]model.UserCandidate, error) {
	return s.recall(ctx, req)
}

type fakeRecommendContentService struct {
	contentservice.ContentService
	unpublished map[int64]struct{}
	err         error
}

func (f *fakeRecommendContentService) GetPostsByIds(_ context.Context, in *contentservice.GetPostsByIdsReq, _ ...grpc.CallOption) (*contentservice.GetPostsByIdsResp, error) {
	if f != nil && f.err != nil {
		return nil, f.err
	}
	posts := make([]*contentservice.PostInfo, 0, len(in.GetPostIds()))
	for _, id := range in.GetPostIds() {
		if f != nil {
			if _, blocked := f.unpublished[id]; blocked {
				continue
			}
		}
		posts = append(posts, &contentservice.PostInfo{Id: id, Status: 1})
	}
	return &contentservice.GetPostsByIdsResp{Posts: posts}, nil
}

type fakeFeatureRepository struct {
	viewer      model.ViewerFeatures
	posts       map[int64]model.PostFeatures
	users       map[int64]model.UserFeatures
	viewerErr   error
	postsErr    error
	usersErr    error
	optedOut    bool
	optedOutErr error
}

func (r *fakeFeatureRepository) LoadViewerFeatures(context.Context, string) (model.ViewerFeatures, error) {
	return r.viewer, r.viewerErr
}

func (r *fakeFeatureRepository) LoadPostFeatures(context.Context, []int64) (map[int64]model.PostFeatures, error) {
	return r.posts, r.postsErr
}

func (r *fakeFeatureRepository) LoadUserFeatures(context.Context, []int64) (map[int64]model.UserFeatures, error) {
	return r.users, r.usersErr
}

func (r *fakeFeatureRepository) IsPersonalizationOptedOut(context.Context, int64) (bool, error) {
	return r.optedOut, r.optedOutErr
}

type memorySnapshotStore struct {
	mu       sync.Mutex
	items    map[string]model.PostSnapshot
	saveErr  error
	loadErr  error
	savedTTL int
}

func (s *memorySnapshotStore) Save(_ context.Context, id string, snapshot model.PostSnapshot, ttl int) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.savedTTL = ttl
	s.items[id] = snapshot
	return nil
}

func (s *memorySnapshotStore) Load(_ context.Context, id string) (model.PostSnapshot, error) {
	if s.loadErr != nil {
		return model.PostSnapshot{}, s.loadErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot, ok := s.items[id]
	if !ok {
		return model.PostSnapshot{}, model.ErrSnapshotMissing
	}
	return snapshot, nil
}

type fakeInferenceRanker struct {
	rank func(context.Context, string, string, []model.PostCandidate) (model.InferenceResult, error)
}

func (r fakeInferenceRanker) Rank(ctx context.Context, requestID, version string, candidates []model.PostCandidate) (model.InferenceResult, error) {
	return r.rank(ctx, requestID, version, candidates)
}

func newTestServiceContext(t *testing.T, now time.Time) *svc.ServiceContext {
	t.Helper()
	codec, err := cursor.New(testCursorSecret, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return &svc.ServiceContext{
		Config: config.Config{
			DefaultPageSize: 2, MaxPageSize: 20, CandidateMultiplier: 4,
			CursorTTLSeconds: 600, RuleModelVersion: "rules-test", MaxPerAuthor: 2,
		},
		CursorCodec:    codec,
		SnapshotStore:  &memorySnapshotStore{items: make(map[string]model.PostSnapshot)},
		Now:            func() time.Time { return now },
		NewSnapshotID:  func() (string, error) { return "snapshot-1", nil },
		ContentService: &fakeRecommendContentService{},
	}
}

func knownPost(postID, authorID int64, category string, quality float64) model.PostCandidate {
	return model.PostCandidate{
		PostID: postID,
		Features: model.PostFeatures{
			Known: true, Available: true, Visibility: "public", AuthorID: authorID,
			Category: category, Quality: quality,
		},
	}
}

func knownUser(userID int64, category string, quality float64) model.UserCandidate {
	return model.UserCandidate{
		UserID: userID,
		Features: model.UserFeatures{
			Known: true, Available: true, Visibility: "public", Category: category, Quality: quality,
		},
	}
}

func TestGetRecommendPostsRunsFullPipelineAndPagesSnapshot(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	serviceContext := newTestServiceContext(t, now)
	requestContext := context.WithValue(context.Background(), struct{ name string }{"trace"}, "preserved")
	serviceContext.Config.ExploreRatio = 0.34
	serviceContext.Config.OnlineInfer = config.OnlineInferConfig{Enabled: true, ModelVersion: "rank-requested", TimeoutMs: 50}
	serviceContext.PostRecallSources = []model.PostRecallSource{
		fakePostSource{name: "hot", recall: func(ctx context.Context, req model.RecallRequest) ([]model.PostCandidate, error) {
			if ctx != requestContext || req.Identity != "u:42" || req.Scene != "home" || req.Limit != 8 {
				t.Errorf("unexpected recall request: %+v", req)
			}
			return []model.PostCandidate{{PostID: 1}, {PostID: 2}, {PostID: 3}, {PostID: 4}}, nil
		}},
		fakePostSource{name: "itemcf", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return []model.PostCandidate{{PostID: 2}, {PostID: 4}}, nil
		}},
		fakePostSource{name: "explore", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return []model.PostCandidate{{PostID: 5}}, nil
		}},
	}
	serviceContext.FeatureRepository = &fakeFeatureRepository{
		viewer: model.ViewerFeatures{
			PositivePostIDs: map[int64]struct{}{},
			NegativePostIDs: map[int64]struct{}{3: {}},
			SeenPostIDs:     map[int64]struct{}{4: {}},
			BlockedAuthors:  map[int64]struct{}{},
		},
		posts: map[int64]model.PostFeatures{
			1: {Known: true, Available: true, Visibility: "public", AuthorID: 10, Category: "tech", Quality: 0.2},
			2: {Known: true, Available: true, Visibility: "public", AuthorID: 20, Category: "tech", Quality: 0.9},
			3: {Known: true, Available: true, Visibility: "public", AuthorID: 30, Category: "sports", Quality: 0.7},
			4: {Known: true, Available: true, Visibility: "public", AuthorID: 40, Category: "news", Quality: 0.8},
			5: {Known: true, Available: true, Visibility: "public", AuthorID: 50, Category: "culture", Quality: 0.7},
		},
	}
	serviceContext.InferenceRanker = fakeInferenceRanker{rank: func(ctx context.Context, requestID, version string, candidates []model.PostCandidate) (model.InferenceResult, error) {
		if ctx.Value(struct{ name string }{"trace"}) != "preserved" || requestID != "request-1" || version != "rank-requested" {
			t.Errorf("inference did not receive request context or metadata")
		}
		if len(candidates) != 3 {
			t.Errorf("inference candidates=%d, want 3", len(candidates))
		}
		return model.InferenceResult{Scores: map[int64]float64{1: 0.5, 2: 0.95, 5: 0.8}, ModelVersion: "rank-v7"}, nil
	}}

	logic := NewGetRecommendPostsLogic(requestContext, serviceContext)
	response, err := logic.GetRecommendPosts(&pb.GetRecommendPostsReq{
		UserId: 42, Scene: "home", RequestId: "request-1", SessionId: "session-1",
		PageSize: 2, ExperimentId: "experiment-a",
	})
	if err != nil {
		t.Fatalf("GetRecommendPosts() error = %v", err)
	}
	if len(response.Posts) != 2 || response.Posts[0].PostId != 2 || response.Posts[1].PostId != 5 {
		t.Fatalf("unexpected first page: %+v", response.Posts)
	}
	if !strings.Contains(response.Posts[0].RecallSource, "hot") || !strings.Contains(response.Posts[0].RecallSource, "itemcf") {
		t.Fatalf("duplicate recall sources were not merged: %+v", response.Posts[0])
	}
	if response.Posts[0].ModelVersion != "rank-v7" || response.Posts[0].Position != 1 || response.Posts[1].Position != 2 {
		t.Fatalf("rank metadata not preserved: %+v", response.Posts)
	}
	if !response.HasMore || response.NextCursor == "" || response.RequestId != "request-1" {
		t.Fatalf("unexpected first page metadata: %+v", response)
	}

	next, err := logic.GetRecommendPosts(&pb.GetRecommendPostsReq{
		UserId: 42, Scene: "home", RequestId: "request-1", SessionId: "session-1",
		PageSize: 2, ExperimentId: "experiment-a", Cursor: response.NextCursor,
	})
	if err != nil {
		t.Fatalf("second page error = %v", err)
	}
	if len(next.Posts) != 1 || next.Posts[0].PostId != 1 || next.Posts[0].Position != 3 || next.HasMore || next.NextCursor != "" {
		t.Fatalf("unexpected second page: %+v", next)
	}
	store := serviceContext.SnapshotStore.(*memorySnapshotStore)
	if store.savedTTL != 600 {
		t.Fatalf("snapshot ttl = %d, want 600", store.savedTTL)
	}
}

func TestGetRecommendPostsMarksPartialRecallDegradation(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	serviceContext.PostRecallSources = []model.PostRecallSource{
		fakePostSource{name: "hot", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return []model.PostCandidate{knownPost(7, 10, "tech", 0.7)}, nil
		}},
		fakePostSource{name: "milvus", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return nil, errors.New("milvus unavailable")
		}},
	}
	serviceContext.FeatureRepository = &fakeFeatureRepository{
		postsErr: errors.New("features down"),
		posts:    map[int64]model.PostFeatures{7: {Known: true, Available: true, Visibility: "public", AuthorID: 10, Quality: 0.7}},
	}
	response, err := NewGetRecommendPostsLogic(context.Background(), serviceContext).GetRecommendPosts(&pb.GetRecommendPostsReq{
		UserId: 1, RequestId: "request-1", PageSize: 2,
	})
	if err != nil {
		t.Fatalf("GetRecommendPosts() error = %v", err)
	}
	if len(response.Posts) != 1 || !strings.Contains(response.Posts[0].ModelVersion, "recall-degraded") ||
		!strings.Contains(response.Posts[0].ModelVersion, "feature-degraded") {
		t.Fatalf("degradation was not explicit: %+v", response.Posts)
	}
}

func TestGetRecommendPostsReturnsUnavailableWhenAllRecallSourcesFail(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	serviceContext.PostRecallSources = []model.PostRecallSource{
		fakePostSource{name: "hot", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return nil, errors.New("redis unavailable")
		}},
		fakePostSource{name: "content", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return nil, errors.New("content unavailable")
		}},
	}
	_, err := NewGetRecommendPostsLogic(context.Background(), serviceContext).GetRecommendPosts(&pb.GetRecommendPostsReq{
		UserId: 1, RequestId: "request-1", PageSize: 2,
	})
	if !errx.Is(err, errx.ServiceUnavailable) {
		t.Fatalf("error = %v, want ServiceUnavailable", err)
	}
}

func TestGetRecommendPostsSkipsPersonalizationWhenOptedOut(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	var itemcfCalled atomic.Bool
	serviceContext.PostRecallSources = []model.PostRecallSource{
		fakePostSource{name: "hot", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return []model.PostCandidate{knownPost(7, 10, "tech", 0.7)}, nil
		}},
		fakePostSource{name: "itemcf", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			itemcfCalled.Store(true)
			return []model.PostCandidate{knownPost(8, 20, "tech", 0.9)}, nil
		}},
		fakePostSource{name: "content_hot", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return []model.PostCandidate{knownPost(9, 30, "news", 0.5)}, nil
		}},
	}
	serviceContext.FeatureRepository = &fakeFeatureRepository{
		optedOut: true,
		viewer: model.ViewerFeatures{
			NegativePostIDs: map[int64]struct{}{7: {}},
			SeenPostIDs:     map[int64]struct{}{9: {}},
		},
		posts: map[int64]model.PostFeatures{
			7: {Known: true, Available: true, Visibility: "public", AuthorID: 10, Category: "tech", Quality: 0.7},
			9: {Known: true, Available: true, Visibility: "public", AuthorID: 30, Category: "news", Quality: 0.5},
		},
	}

	response, err := NewGetRecommendPostsLogic(context.Background(), serviceContext).GetRecommendPosts(&pb.GetRecommendPostsReq{
		UserId: 42, RequestId: "request-1", PageSize: 5,
	})
	if err != nil {
		t.Fatalf("GetRecommendPosts() error = %v", err)
	}
	if itemcfCalled.Load() {
		t.Fatal("personalized recall source was called after opt-out")
	}
	if len(response.Posts) != 2 {
		t.Fatalf("expected both rule-based candidates regardless of viewer features, got %+v", response.Posts)
	}
	for _, post := range response.Posts {
		if !strings.Contains(post.ModelVersion, "personalization-disabled") {
			t.Fatalf("opt-out was not marked: %+v", post)
		}
	}
}

func TestGetRecommendPostsFallsBackWhenInferenceTimesOut(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	serviceContext.Config.OnlineInfer = config.OnlineInferConfig{Enabled: true, ModelVersion: "rank-v1", TimeoutMs: 5}
	serviceContext.PostRecallSources = []model.PostRecallSource{
		fakePostSource{name: "hot", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return []model.PostCandidate{knownPost(8, 20, "tech", 0.5)}, nil
		}},
	}
	serviceContext.InferenceRanker = fakeInferenceRanker{rank: func(ctx context.Context, _ string, _ string, _ []model.PostCandidate) (model.InferenceResult, error) {
		<-ctx.Done()
		return model.InferenceResult{}, ctx.Err()
	}}
	response, err := NewGetRecommendPostsLogic(context.Background(), serviceContext).GetRecommendPosts(&pb.GetRecommendPostsReq{
		UserId: 1, RequestId: "request-timeout", PageSize: 2,
	})
	if err != nil {
		t.Fatalf("GetRecommendPosts() error = %v", err)
	}
	if len(response.Posts) != 1 || !strings.Contains(response.Posts[0].ModelVersion, "infer-timeout") {
		t.Fatalf("timeout fallback was not explicit: %+v", response.Posts)
	}
}

func TestGetRecommendPostsAnonymousUsesRuleOnlySources(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	var itemcfCalled atomic.Bool
	var followCalled atomic.Bool
	serviceContext.PostRecallSources = []model.PostRecallSource{
		fakePostSource{name: "hot", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return []model.PostCandidate{knownPost(1, 10, "tech", 0.7)}, nil
		}},
		fakePostSource{name: "itemcf", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			itemcfCalled.Store(true)
			return nil, nil
		}},
		fakePostSource{name: "follow", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			followCalled.Store(true)
			return nil, nil
		}},
	}
	serviceContext.FeatureRepository = &fakeFeatureRepository{
		posts: map[int64]model.PostFeatures{
			1: {Known: true, Available: true, Visibility: "public", AuthorID: 10, Category: "tech", Quality: 0.7},
		},
	}
	response, err := NewGetRecommendPostsLogic(context.Background(), serviceContext).GetRecommendPosts(&pb.GetRecommendPostsReq{
		AnonymousId: "device-1", RequestId: "request-1", PageSize: 5,
	})
	if err != nil {
		t.Fatalf("GetRecommendPosts() error = %v", err)
	}
	if itemcfCalled.Load() || followCalled.Load() {
		t.Fatal("personalized recall sources must not be used for anonymous users (DISC-031)")
	}
	if len(response.Posts) != 1 || response.Posts[0].PostId != 1 {
		t.Fatalf("anonymous cold-start should return only rule-based posts: %+v", response.Posts)
	}
}

func TestGetRecommendPostsRejectsTamperedCursorBeforeRecall(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	var called atomic.Bool
	serviceContext.PostRecallSources = []model.PostRecallSource{
		fakePostSource{name: "hot", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			called.Store(true)
			return nil, nil
		}},
	}
	_, err := NewGetRecommendPostsLogic(context.Background(), serviceContext).GetRecommendPosts(&pb.GetRecommendPostsReq{
		UserId: 1, RequestId: "request-1", PageSize: 2, Cursor: "tampered.cursor",
	})
	if !errx.Is(err, errx.ParamError) {
		t.Fatalf("error = %v, want ParamError", err)
	}
	if called.Load() {
		t.Fatal("recall source was called for an invalid cursor")
	}
}

func TestGetSimilarPostsUsesMultiRecallAndExcludesSeed(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	serviceContext.SimilarPostSources = []model.PostRecallSource{
		fakePostSource{name: "itemcf", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return []model.PostCandidate{knownPost(10, 1, "tech", 1), knownPost(11, 2, "tech", 0.8)}, nil
		}},
		fakePostSource{name: "milvus", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return []model.PostCandidate{knownPost(11, 2, "tech", 0.8), knownPost(12, 3, "culture", 0.7)}, nil
		}},
	}
	response, err := NewGetSimilarPostsLogic(context.Background(), serviceContext).GetSimilarPosts(&pb.GetSimilarPostsReq{
		PostId: 10, Limit: 2, RequestId: "similar-1", ExperimentId: "exp-1",
	})
	if err != nil {
		t.Fatalf("GetSimilarPosts() error = %v", err)
	}
	if len(response.Posts) != 2 || response.Posts[0].PostId != 11 || response.Posts[1].PostId != 12 {
		t.Fatalf("unexpected similar posts: %+v", response.Posts)
	}
	if !strings.Contains(response.Posts[0].RecallSource, "itemcf") || !strings.Contains(response.Posts[0].RecallSource, "milvus") {
		t.Fatalf("similar recall sources not merged: %+v", response.Posts[0])
	}
}

func TestGetRecommendUsersDeduplicatesAndFiltersCurrentUser(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	serviceContext.UserRecallSources = []model.UserRecallSource{
		fakeUserSource{name: "mutual", recall: func(context.Context, model.RecallRequest) ([]model.UserCandidate, error) {
			return []model.UserCandidate{knownUser(42, "tech", 1), knownUser(7, "tech", 0.8)}, nil
		}},
		fakeUserSource{name: "interest", recall: func(context.Context, model.RecallRequest) ([]model.UserCandidate, error) {
			return []model.UserCandidate{knownUser(7, "tech", 0.8), knownUser(8, "culture", 0.7)}, nil
		}},
	}
	response, err := NewGetRecommendUsersLogic(context.Background(), serviceContext).GetRecommendUsers(&pb.GetRecommendUsersReq{
		UserId: 42, RequestId: "users-1", Limit: 2, ExperimentId: "exp-1",
	})
	if err != nil {
		t.Fatalf("GetRecommendUsers() error = %v", err)
	}
	if len(response.Users) != 2 || response.Users[0].UserId != 7 || response.Users[1].UserId != 8 {
		t.Fatalf("unexpected users: %+v", response.Users)
	}
	if !strings.Contains(response.Users[0].RecallSource, "mutual") || !strings.Contains(response.Users[0].RecallSource, "interest") {
		t.Fatalf("user recall sources not merged: %+v", response.Users[0])
	}
}

func TestEnforceAuthorQuotaEvery20Window(t *testing.T) {
	posts := make([]model.PostCandidate, 0, 25)
	for i := int64(1); i <= 20; i++ {
		posts = append(posts, model.PostCandidate{PostID: i, AuthorID: i + 100})
	}
	for i := int64(0); i < 5; i++ {
		posts = append(posts, model.PostCandidate{PostID: 1000 + i, AuthorID: 999})
	}

	quota := enforceAuthorQuota(posts, 2)
	windowCounts := map[int64]int{}
	for _, post := range quota[:20] {
		windowCounts[post.AuthorID]++
	}
	for author, count := range windowCounts {
		if count > 2 {
			t.Fatalf("author %d appears %d times in first 20 window", author, count)
		}
	}
}

func TestEnforceAuthorQuotaSkippedBelowTenAuthors(t *testing.T) {
	posts := []model.PostCandidate{
		{PostID: 1, AuthorID: 10}, {PostID: 2, AuthorID: 10}, {PostID: 3, AuthorID: 10},
	}
	if got := enforceAuthorQuota(posts, 2); len(got) != 3 {
		t.Fatalf("quota should not apply below 10 distinct authors, got %d", len(got))
	}
}

func TestSeenPostsReenterWhenCandidatesInsufficient(t *testing.T) {
	candidates := []model.PostCandidate{
		{PostID: 1, Features: model.PostFeatures{Known: true, Available: true, Visibility: "public", AuthorID: 10}},
		{PostID: 2, Features: model.PostFeatures{Known: true, Available: true, Visibility: "public", AuthorID: 20}},
	}
	viewer := model.ViewerFeatures{
		SeenPostIDs:     map[int64]struct{}{1: {}},
		PositivePostIDs: map[int64]struct{}{},
		NegativePostIDs: map[int64]struct{}{},
		BlockedAuthors:  map[int64]struct{}{},
	}
	result, degraded, err := enrichAndFilterPosts(context.Background(), &fakeFeatureRepository{
		viewer: viewer,
		posts: map[int64]model.PostFeatures{
			1: {Known: true, Available: true, Visibility: "public", AuthorID: 10},
			2: {Known: true, Available: true, Visibility: "public", AuthorID: 20},
		},
	}, "u:42", candidates, 0, false)
	if err != nil {
		t.Fatalf("enrichAndFilterPosts() error = %v", err)
	}
	if len(result) != 1 || result[0].PostID != 2 {
		t.Fatalf("expected only unseen post without re-entry, got %+v", result)
	}

	// 全部候选都被曝光 → 候选不足，允许重新进入并标记原因
	onlySeen := []model.PostCandidate{
		{PostID: 1, Features: model.PostFeatures{Known: true, Available: true, Visibility: "public", AuthorID: 10}},
	}
	result, _, err = enrichAndFilterPosts(context.Background(), &fakeFeatureRepository{
		viewer: viewer,
		posts: map[int64]model.PostFeatures{
			1: {Known: true, Available: true, Visibility: "public", AuthorID: 10},
		},
	}, "u:42", onlySeen, 0, false)
	if err != nil {
		t.Fatalf("enrichAndFilterPosts() error = %v", err)
	}
	if len(result) != 1 || result[0].PostID != 1 ||
		!strings.Contains(result[0].Reason, "re-entered after exposure window") {
		t.Fatalf("seen post did not re-enter with reason: %+v", result)
	}
	if degraded {
		t.Fatal("seen re-entry should not mark feature repository degraded")
	}
}

func TestGetRecommendPostsDropsUnpublishedContent(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	serviceContext.PostRecallSources = []model.PostRecallSource{
		fakePostSource{name: "hot", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return []model.PostCandidate{knownPost(1, 10, "tech", 0.9), knownPost(2, 20, "tech", 0.8)}, nil
		}},
	}
	serviceContext.FeatureRepository = &fakeFeatureRepository{
		posts: map[int64]model.PostFeatures{
			1: {Known: true, Available: true, Visibility: "public", AuthorID: 10, Quality: 0.9},
			2: {Known: true, Available: true, Visibility: "public", AuthorID: 20, Quality: 0.8},
		},
	}
	serviceContext.ContentService = &fakeRecommendContentService{unpublished: map[int64]struct{}{2: {}}}
	resp, err := NewGetRecommendPostsLogic(context.Background(), serviceContext).GetRecommendPosts(&pb.GetRecommendPostsReq{
		UserId: 7, Scene: "home", RequestId: "req-vis", SessionId: "sess-vis", PageSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Posts) != 1 || resp.Posts[0].PostId != 1 {
		t.Fatalf("unpublished post leaked: %+v", resp.Posts)
	}
}

func TestGetRecommendPostsVisibilityUnavailableFailsClosed(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	serviceContext.PostRecallSources = []model.PostRecallSource{
		fakePostSource{name: "hot", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return []model.PostCandidate{knownPost(1, 10, "tech", 0.9)}, nil
		}},
	}
	serviceContext.FeatureRepository = &fakeFeatureRepository{
		posts: map[int64]model.PostFeatures{1: {Known: true, Available: true, Visibility: "public", AuthorID: 10, Quality: 0.9}},
	}
	serviceContext.ContentService = &fakeRecommendContentService{err: errors.New("content down")}
	_, err := NewGetRecommendPostsLogic(context.Background(), serviceContext).GetRecommendPosts(&pb.GetRecommendPostsReq{
		UserId: 7, Scene: "home", RequestId: "req-vis-fail", SessionId: "sess-vis-fail", PageSize: 10,
	})
	if err == nil || !errx.Is(err, errx.ServiceUnavailable) {
		t.Fatalf("want visibility fail-closed, got %v", err)
	}
}

func TestGetRecommendPostsTreatsOptOutReadFailureAsDisabled(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	personalizedCalled := false
	serviceContext.PostRecallSources = []model.PostRecallSource{
		fakePostSource{name: "hot", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return []model.PostCandidate{knownPost(1, 10, "tech", 0.9)}, nil
		}},
		fakePostSource{name: "personalized", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			personalizedCalled = true
			return []model.PostCandidate{knownPost(99, 99, "tech", 0.9)}, nil
		}},
	}
	serviceContext.FeatureRepository = &fakeFeatureRepository{
		optedOutErr: errors.New("redis down"),
		posts:       map[int64]model.PostFeatures{1: {Known: true, Available: true, Visibility: "public", AuthorID: 10, Quality: 0.9}},
	}
	resp, err := NewGetRecommendPostsLogic(context.Background(), serviceContext).GetRecommendPosts(&pb.GetRecommendPostsReq{
		UserId: 7, Scene: "home", RequestId: "req-opt", SessionId: "sess-opt", PageSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if personalizedCalled {
		t.Fatal("personalized recall ran after opt-out read failure")
	}
	if len(resp.Posts) != 1 || !strings.Contains(resp.Posts[0].ModelVersion, "personalization-disabled") {
		t.Fatalf("opt-out failure was not marked: %+v", resp.Posts)
	}
}
