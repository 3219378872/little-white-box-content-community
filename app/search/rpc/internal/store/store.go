package store

import "context"

// MaxResultWindow 与 ES 默认 index.max_result_window 一致，分页不能越过该窗口。
const MaxResultWindow = 10000

// PostQuery 是帖子搜索参数；SortBy 2=最新、3=最热，其余按相关度。
type PostQuery struct {
	Keyword  string
	Page     int32
	PageSize int32
	SortBy   int32
	Tags     []string
}

// Post 是索引中的帖子命中；标题与摘要会在逻辑层用内容服务的权威数据回填。
type Post struct {
	ID               int64
	AuthorID         int64
	Title            string
	ContentHighlight string
	LikeCount        int64
	CommentCount     int64
	CreatedAt        int64
}

// PostResult 是一页帖子命中及 ES 统计的总数（最多统计到 MaxResultWindow）。
type PostResult struct {
	Posts []Post
	Total int64
}

// Tag 是标签及其已发布帖子数。
type Tag struct {
	Name      string
	PostCount int64
}

// Store 是搜索读路径的存储抽象，便于用 TagCache 装饰或在测试中替换。
type Store interface {
	Health(ctx context.Context) error
	SearchPosts(ctx context.Context, query PostQuery) (PostResult, error)
	SearchTags(ctx context.Context, keyword string, limit int32) ([]Tag, error)
	HotSearches(ctx context.Context, limit int32) ([]string, error)
}
