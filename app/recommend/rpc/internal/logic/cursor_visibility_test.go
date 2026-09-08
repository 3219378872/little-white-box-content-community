package logic

import (
	"context"
	"testing"
	"time"

	"esx/app/recommend/rpc/internal/cursor"
	"esx/app/recommend/rpc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecommendationCursorAdvancesPastInvisibleRows(t *testing.T) {
	s := newTestServiceContext(t, time.Unix(1800000000, 0))
	l := NewGetRecommendPostsLogic(context.Background(), s)
	binding := cursor.Binding{IdentityHash: cursor.IdentityHash("u:1"), RequestID: "request", Scene: "home", PageSize: 2}
	posts := []model.RankedPost{{PostID: 1}, {PostID: 2}, {PostID: 3}, {PostID: 4}, {PostID: 5}, {PostID: 6}}
	first, err := l.firstPage(posts, 2, binding)
	require.NoError(t, err)
	s.ContentService.(*fakeRecommendContentService).unpublished = map[int64]struct{}{3: {}}
	second, err := l.pageFromCursor(first.NextCursor, 2, binding)
	require.NoError(t, err)
	require.Len(t, second.Posts, 2)
	assert.Equal(t, int64(4), second.Posts[0].PostId)
	assert.Equal(t, int64(5), second.Posts[1].PostId)
	third, err := l.pageFromCursor(second.NextCursor, 2, binding)
	require.NoError(t, err)
	require.Len(t, third.Posts, 1)
	assert.Equal(t, int64(6), third.Posts[0].PostId)
	assert.False(t, third.HasMore)
}
