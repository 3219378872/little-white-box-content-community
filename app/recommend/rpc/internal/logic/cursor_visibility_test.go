package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"esx/app/recommend/rpc/internal/cursor"
	"esx/app/recommend/rpc/internal/model"
	"esx/app/recommend/rpc/xiaobaihe/recommend/pb"
	"esx/pkg/errx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecommendationCursorAdvancesPastInvisibleRows(t *testing.T) {
	s := newTestServiceContext(t, time.Unix(1800000000, 0))
	s.FeatureRepository = &fakeFeatureRepository{}
	l := NewGetRecommendPostsLogic(context.Background(), s)
	binding := cursor.Binding{IdentityHash: cursor.IdentityHash("u:1"), RequestID: "request", Scene: "home", PageSize: 2}
	posts := []model.RankedPost{{PostID: 1}, {PostID: 2}, {PostID: 3}, {PostID: 4}, {PostID: 5}, {PostID: 6}}
	first, err := l.firstPage(posts, 2, binding, false)
	require.NoError(t, err)
	s.ContentService.(*fakeRecommendContentService).unpublished = map[int64]struct{}{3: {}}
	second, err := l.pageFromCursor(first.NextCursor, 2, binding, "u:1")
	require.NoError(t, err)
	require.Len(t, second.Posts, 2)
	assert.Equal(t, int64(4), second.Posts[0].PostId)
	assert.Equal(t, int64(5), second.Posts[1].PostId)
	third, err := l.pageFromCursor(second.NextCursor, 2, binding, "u:1")
	require.NoError(t, err)
	require.Len(t, third.Posts, 1)
	assert.Equal(t, int64(6), third.Posts[0].PostId)
	assert.False(t, third.HasMore)
}

func TestRecommendationCursorRechecksExplicitHides(t *testing.T) {
	s := newTestServiceContext(t, time.Unix(1800000000, 0))
	repo := &fakeFeatureRepository{}
	s.FeatureRepository = repo
	l := NewGetRecommendPostsLogic(context.Background(), s)
	binding := cursor.Binding{IdentityHash: cursor.IdentityHash("u:1"), RequestID: "request", Scene: "home", PageSize: 2}
	first, err := l.firstPage([]model.RankedPost{{PostID: 1}, {PostID: 2}, {PostID: 3}, {PostID: 4}, {PostID: 5}, {PostID: 6}}, 2, binding, false)
	require.NoError(t, err)
	repo.viewer.NegativePostIDs = map[int64]struct{}{3: {}}
	second, err := l.pageFromCursor(first.NextCursor, 2, binding, "u:1")
	require.NoError(t, err)
	require.Len(t, second.Posts, 2)
	assert.Equal(t, int64(4), second.Posts[0].PostId)
	assert.Equal(t, int64(5), second.Posts[1].PostId)
	require.NotEmpty(t, second.NextCursor)
	third, err := l.pageFromCursor(second.NextCursor, 2, binding, "u:1")
	require.NoError(t, err)
	require.Len(t, third.Posts, 1)
	assert.Equal(t, int64(6), third.Posts[0].PostId)
	assert.False(t, third.HasMore)
	// All remaining candidates hidden is a successful end of the chain.
	repo.viewer.NegativePostIDs = map[int64]struct{}{3: {}, 4: {}, 5: {}, 6: {}}
	empty, err := l.pageFromCursor(first.NextCursor, 2, binding, "u:1")
	require.NoError(t, err)
	require.Empty(t, empty.Posts)
	assert.False(t, empty.HasMore)
	assert.Empty(t, empty.NextCursor)
}

func TestRecommendationCursorHiddenFeedbackUnavailable(t *testing.T) {
	s := newTestServiceContext(t, time.Unix(1800000000, 0))
	l := NewGetRecommendPostsLogic(context.Background(), s)
	binding := cursor.Binding{IdentityHash: cursor.IdentityHash("u:1"), RequestID: "request", Scene: "home", PageSize: 1}
	first, err := l.firstPage([]model.RankedPost{{PostID: 1}, {PostID: 2}}, 1, binding, false)
	require.NoError(t, err)
	for _, repo := range []model.FeatureRepository{nil, &fakeFeatureRepository{viewerErr: errors.New("feedback unavailable")}} {
		s.FeatureRepository = repo
		page, err := l.pageFromCursor(first.NextCursor, 1, binding, "u:1")
		require.Error(t, err)
		require.Nil(t, page)
	}
	// Anonymous paging must not query or establish any viewer profile.
	binding.IdentityHash = cursor.IdentityHash("a:anon")
	first, err = l.firstPage([]model.RankedPost{{PostID: 1}, {PostID: 2}}, 1, binding, false)
	require.NoError(t, err)
	page, err := l.pageFromCursor(first.NextCursor, 1, binding, "a:anon")
	require.NoError(t, err)
	require.Len(t, page.Posts, 1)
}

func TestRecommendationCursorInvalidatesPersonalizationAfterOptOut(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "preference unavailable"}[unknown], func(t *testing.T) {
			s := newTestServiceContext(t, time.Unix(1800000000, 0))
			repo := &fakeFeatureRepository{}
			s.FeatureRepository = repo
			s.PostRecallSources = []model.PostRecallSource{fakePostSource{name: "hot", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
				return []model.PostCandidate{knownPost(1, 11, "a", 0.9), knownPost(2, 12, "b", 0.8), knownPost(3, 13, "c", 0.7)}, nil
			}}}
			l := NewGetRecommendPostsLogic(context.Background(), s)
			req := &pb.GetRecommendPostsReq{UserId: 1, RequestId: "request", Scene: "home", PageSize: 2}
			first, err := l.GetRecommendPosts(req)
			require.NoError(t, err)
			require.NotEmpty(t, first.NextCursor)
			repo.optedOut = true
			if unknown {
				repo.optedOutErr = errors.New("preference unavailable")
			}
			req.Cursor = first.NextCursor
			page, err := l.GetRecommendPosts(req)
			require.True(t, errx.Is(err, errx.ParamError), "%v", err)
			require.Nil(t, page)
			// A fresh rule-only chain remains pageable while opted out or unknown.
			req.Cursor = ""
			first, err = l.GetRecommendPosts(req)
			require.NoError(t, err)
			require.NotEmpty(t, first.NextCursor)
			req.Cursor = first.NextCursor
			page, err = l.GetRecommendPosts(req)
			require.NoError(t, err)
			require.Len(t, page.Posts, 1)
		})
	}
}
