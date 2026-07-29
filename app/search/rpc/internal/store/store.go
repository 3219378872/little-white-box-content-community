package store

import "context"

const MaxResultWindow = 10000

type PostQuery struct {
	Keyword  string
	Page     int32
	PageSize int32
	SortBy   int32
	Tags     []string
}

type Post struct {
	ID               int64
	AuthorID         int64
	Title            string
	ContentHighlight string
	LikeCount        int64
	CommentCount     int64
	CreatedAt        int64
}

type PostResult struct {
	Posts []Post
	Total int64
}

type Tag struct {
	Name      string
	PostCount int64
}

type Store interface {
	Health(ctx context.Context) error
	SearchPosts(ctx context.Context, query PostQuery) (PostResult, error)
	SearchTags(ctx context.Context, keyword string, limit int32) ([]Tag, error)
	HotSearches(ctx context.Context, limit int32) ([]string, error)
}
