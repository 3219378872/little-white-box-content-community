package logic

import "esx/pkg/pageutil"

const (
	defaultPageSize = int(pageutil.DefaultPageSize)
	maxPageSize     = int(pageutil.ContentMaxPageSize)
	// maxTagListLimit 标签列表返回上限（防御性：GetTags RPC 当前无外部入口）。
	maxTagListLimit = 100
)

func normalizePage(page, pageSize int) (int, int) {
	return int(pageutil.ClampPage(int32(page))), int(pageutil.ClampPageSize(int32(pageSize)))
}

func normalizePageSize(pageSize int) int {
	return int(pageutil.ClampPageSize(int32(pageSize)))
}

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
