package logic

import "esx/pkg/pageutil"

const (
	defaultPageSize = int(pageutil.DefaultPageSize)
	maxPageSize     = int(pageutil.ContentMaxPageSize)
	// maxTagListLimit 标签列表返回上限（GetTags 供 Assistant 等内部调用方使用）。
	maxTagListLimit = 100
)

// normalizePage 把页码与页大小规范到默认值与上限之内。
func normalizePage(page, pageSize int) (int, int) {
	return int(pageutil.ClampPage(int32(page))), int(pageutil.ClampPageSize(int32(pageSize)))
}

// normalizePageSize 把页大小规范到默认值与上限之内。
func normalizePageSize(pageSize int) int {
	return int(pageutil.ClampPageSize(int32(pageSize)))
}

// normalizeSortBy 规范排序方式；按浏览量排序只对全局列表开放，其余情况回退为最新。
func normalizeSortBy(sortBy int, allowViewed bool) int {
	switch sortBy {
	case 1: // SortByLatest
		return sortBy
	case 2: // SortByHot
		return sortBy
	case 3: // SortByViewed（仅全局列表）
		if allowViewed {
			return sortBy
		}
	}
	return 1
}
