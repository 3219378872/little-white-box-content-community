package model

// Sort modes used by PostModel query methods.
// Values map to proto GetUserPostsReq.sort_by / GetPostListReq.sort_by.
const (
	SortByLatest = 1 // 最新（created_at desc）—— 默认
	SortByHot    = 2 // 热门（like_count desc, created_at desc）
	SortByViewed = 3 // 浏览量（view_count desc）仅用于 FindList
)
