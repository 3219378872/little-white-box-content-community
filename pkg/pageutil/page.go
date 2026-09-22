// Package pageutil clamps pagination so gateway and RPC services share one policy:
// non-positive page → 1; non-positive page size → default; size above max → max.
package pageutil

import "errors"

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

// MaxResultWindow matches the user-search offset ceiling. Offsets at or beyond
// it are rejected instead of being sent to SQL.
const MaxResultWindow int64 = 10_000

// ErrPageWindow is returned when a page would read past MaxResultWindow.
var ErrPageWindow = errors.New("pageutil: result window exceeds limit")

// PageOffset multiplies in int64 and rejects windows past MaxResultWindow.
func PageOffset(page, pageSize int32) (int64, error) {
	if page <= 0 || pageSize <= 0 {
		return 0, ErrPageWindow
	}
	offset := int64(page-1) * int64(pageSize)
	if offset >= MaxResultWindow || offset+int64(pageSize) > MaxResultWindow {
		return 0, ErrPageWindow
	}
	return offset, nil
}
