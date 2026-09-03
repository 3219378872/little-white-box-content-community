// Package pageutil clamps pagination so gateway and RPC services share one policy:
// non-positive page → 1; non-positive page size → default; size above max → max.
package pageutil

const (
	DefaultPageSize        int32 = 20
	ContentMaxPageSize     int32 = 50
	InteractionMaxPageSize int32 = 100
)

func ClampPage(page int32) int32 {
	if page <= 0 {
		return 1
	}
	return page
}

// ClampPageSize is the content-list clamp (default 20, max 50).
func ClampPageSize(pageSize int32) int32 {
	return ClampPageSizeTo(pageSize, DefaultPageSize, ContentMaxPageSize)
}

func ClampPageSizeTo(pageSize, def, max int32) int32 {
	if pageSize <= 0 {
		return def
	}
	if pageSize > max {
		return max
	}
	return pageSize
}
