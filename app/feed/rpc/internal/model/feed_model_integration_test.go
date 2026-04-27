//go:build integration

package model

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeedInboxModel_BatchInsertIgnore_Deduplicates(t *testing.T) {
	conn, cleanup := newFeedTestDB(t)
	defer cleanup()
	feedModel := NewFeedInboxModel(conn)
	rows := []*FeedInbox{
		{UserId: 1, AuthorId: 9, PostId: 1001, CreatedAt: 1000},
		{UserId: 1, AuthorId: 9, PostId: 1001, CreatedAt: 1000},
	}
	affected, err := feedModel.BatchInsertIgnore(context.Background(), rows)

	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)
}

func TestFeedInboxModel_FindByUserBefore_OrderStable(t *testing.T) {
	conn, cleanup := newFeedTestDB(t)
	defer cleanup()
	feedModel := NewFeedInboxModel(conn)
	_, err := feedModel.BatchInsertIgnore(context.Background(), []*FeedInbox{
		{UserId: 1, AuthorId: 9, PostId: 1001, CreatedAt: 1000},
		{UserId: 1, AuthorId: 9, PostId: 1002, CreatedAt: 1000},
		{UserId: 1, AuthorId: 9, PostId: 1003, CreatedAt: 999},
	})
	require.NoError(t, err)
	rows, err := feedModel.FindByUserBefore(context.Background(), 1, math.MaxInt64, math.MaxInt64, 3)

	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, []int64{1002, 1001, 1003}, []int64{rows[0].PostId, rows[1].PostId, rows[2].PostId})
}
