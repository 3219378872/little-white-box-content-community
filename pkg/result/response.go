package result

import "errors"

// bizError 用于从 error 中提取业务错误码（避免循环依赖，通过接口解耦）
type bizError interface {
	error
	BizCode() int
}

// extractCode 从 error 中提取业务错误码，提取失败返回 -1
func extractCode(err error) int {
	if be, ok := errors.AsType[bizError](err); ok {
		return be.BizCode()
	}
	return -1
}

// FailWithError 从 error 提取业务码，统一包装为失败响应（配合 errx 使用）
func FailWithError(err error) *Result[any] {
	return &Result[any]{
		Code:    extractCode(err),
		Message: err.Error(),
	}
}

// Result 统一响应结构体
type Result[T any] struct {
	Code    int    `json:"code"`    // 业务状态码
	Message string `json:"message"` // 提示信息
	Data    T      `json:"data"`    // 响应数据
}

// Success 成功响应
func Success[T any](data T) *Result[T] {
	return &Result[T]{
		Code:    0,
		Message: "success",
		Data:    data,
	}
}

// Fail 失败响应
func Fail[T any](code int, message string) *Result[T] {
	return &Result[T]{
		Code:    code,
		Message: message,
	}
}

// PageData 分页数据结构
type PageData[T any] struct {
	List     []T   `json:"list"`     // 数据列表
	Total    int64 `json:"total"`    // 总数
	Page     int64 `json:"page"`     // 当前页
	PageSize int64 `json:"pageSize"` // 每页数量
}

// SuccessPage 分页成功响应
func SuccessPage[T any](list []T, total, page, pageSize int64) *Result[PageData[T]] {
	return &Result[PageData[T]]{
		Code:    0,
		Message: "success",
		Data: PageData[T]{
			List:     list,
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		},
	}
}
