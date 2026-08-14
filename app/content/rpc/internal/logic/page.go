package logic

// Page bounds shared by paged content reads. Defaults preserve the historical
// behavior of every paged list logic in this package.
const (
	defaultPageSize = 20
	maxPageSize     = 50
	// maxTagListLimit 标签列表返回上限（防御性：GetTags RPC 当前无外部入口）。
	maxTagListLimit = 100
)

// normalizePage clamps request pagination to the package defaults:
// non-positive pages become 1, and page sizes outside 1..maxPageSize become
// defaultPageSize.
func normalizePage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > maxPageSize {
		pageSize = defaultPageSize
	}
	return page, pageSize
}
