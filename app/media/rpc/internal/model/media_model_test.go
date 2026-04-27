package model

import (
	"context"
	"testing"
)

// FindByIds 对空 ids 必须短路返回，避免发出无意义的 `WHERE id IN ()` 空 SQL。
// DB 路径由集成测试（Task 19 BatchGetMedia）覆盖。
//
// 这里绕过 NewMediaModel —— 它通过 sqlc.NewConn 立刻 MustNewRedis，
// 需要真实 Redis。空分支在任何 CachedConn 方法之前返回，故零值 embedding 足矣。
func TestFindByIds_EmptyIdsReturnsEmptySliceWithoutQuerying(t *testing.T) {
	m := &customMediaModel{
		defaultMediaModel: &defaultMediaModel{table: "`media`"},
	}

	for _, ids := range [][]int64{nil, {}} {
		rows, err := m.FindByIds(context.Background(), ids)
		if err != nil {
			t.Fatalf("ids=%v: unexpected err: %v", ids, err)
		}
		if rows == nil {
			t.Fatalf("ids=%v: expected non-nil empty slice, got nil", ids)
		}
		if len(rows) != 0 {
			t.Fatalf("ids=%v: expected empty slice, got %d rows", ids, len(rows))
		}
	}
}
