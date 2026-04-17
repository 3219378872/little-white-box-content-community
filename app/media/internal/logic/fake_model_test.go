package logic

import (
	"context"
	"database/sql"

	"esx/app/media/internal/model"
)

// fakeMediaModel 提供 model.MediaModel 的可注入替身；未设置的方法调用会 panic，
// 便于测试暴露调用了预期之外的方法。
type fakeMediaModel struct {
	findOneFn   func(ctx context.Context, id int64) (*model.Media, error)
	findByIdsFn func(ctx context.Context, ids []int64) ([]*model.Media, error)
	updateFn    func(ctx context.Context, data *model.Media) error
	insertFn    func(ctx context.Context, data *model.Media) (sql.Result, error)
	deleteFn    func(ctx context.Context, id int64) error
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
