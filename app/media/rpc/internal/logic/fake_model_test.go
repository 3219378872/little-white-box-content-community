package logic

import (
	"context"
	"database/sql"
	"esx/app/media/rpc/internal/model"
)

// fakeResult 为测试提供可控的 sql.Result
type fakeResult struct {
	lastInsertId int64
	rowsAffected int64
}

func (f *fakeResult) LastInsertId() (int64, error) {
	return f.lastInsertId, nil
}

func (f *fakeResult) RowsAffected() (int64, error) {
	return f.rowsAffected, nil
}

// fakeMediaModel 提供 model.MediaModel 的可注入替身；未设置的方法调用会 panic，
// 便于测试暴露调用了预期之外的方法。
type fakeMediaModel struct {
	findOneFn      func(ctx context.Context, id int64) (*model.Media, error)
	findByIdsFn    func(ctx context.Context, ids []int64) ([]*model.Media, error)
	updateFn       func(ctx context.Context, data *model.Media) error
	updateStatusFn func(ctx context.Context, id int64, expectedStatus, newStatus int64) (sql.Result, error)
	insertFn       func(ctx context.Context, data *model.Media) (sql.Result, error)
	deleteFn       func(ctx context.Context, id int64) error
}

func (f *fakeMediaModel) FindOne(ctx context.Context, id int64) (*model.Media, error) {
	if f.findOneFn == nil {
		panic("fakeMediaModel: FindOne not configured")
	}
	return f.findOneFn(ctx, id)
}

func (f *fakeMediaModel) FindByIds(ctx context.Context, ids []int64) ([]*model.Media, error) {
	if f.findByIdsFn == nil {
		panic("fakeMediaModel: FindByIds not configured")
	}
	return f.findByIdsFn(ctx, ids)
}

func (f *fakeMediaModel) Update(ctx context.Context, data *model.Media) error {
	if f.updateFn == nil {
		panic("fakeMediaModel: Update not configured")
	}
	return f.updateFn(ctx, data)
}

func (f *fakeMediaModel) UpdateStatus(ctx context.Context, id int64, expectedStatus, newStatus int64) (sql.Result, error) {
	if f.updateStatusFn == nil {
		panic("fakeMediaModel: UpdateStatus not configured")
	}
	return f.updateStatusFn(ctx, id, expectedStatus, newStatus)
}

func (f *fakeMediaModel) Insert(ctx context.Context, data *model.Media) (sql.Result, error) {
	if f.insertFn == nil {
		panic("fakeMediaModel: Insert not configured")
	}
	return f.insertFn(ctx, data)
}

func (f *fakeMediaModel) Delete(ctx context.Context, id int64) error {
	if f.deleteFn == nil {
		panic("fakeMediaModel: Delete not configured")
	}
	return f.deleteFn(ctx, id)
}
