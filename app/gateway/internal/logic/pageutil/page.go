// Package pageutil 提供与内容 RPC clamp 语义一致的页大小归一化，
// 保证 gateway 透传的 pageSize 与内容服务实际使用值一致（响应元数据不回传原始值）。
package pageutil

// 与 app/content/rpc/internal/logic 的 normalizePage 保持一致：
// 默认 20、上限 50。
const (
	defaultPageSize = 20
	MaxPageSize     = 50
)

// ClampPageSize 将请求页大小归一化到内容 RPC 的 clamp 语义
// （非正数取默认 20；超过上限取 50）。
func ClampPageSize(pageSize int32) int32 {
	if pageSize <= 0 {
		return defaultPageSize
	}
	if pageSize > MaxPageSize {
		return MaxPageSize
	}
	return pageSize
}
