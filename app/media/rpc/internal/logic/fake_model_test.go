package logic

import (
	"context"
	"database/sql"
	"esx/app/media/rpc/internal/model"
	"esx/pkg/idempotencyx"
	"esx/pkg/outboxx"
)

// fakeMediaModel 提供 model.MediaModel 的可注入替身；未设置的方法调用会 panic，
// 便于测试暴露调用了预期之外的方法。
type fakeMediaModel struct {
	findOneFn      func(ctx context.Context, id int64) (*model.Media, error)
	findByIdsFn    func(ctx context.Context, ids []int64) ([]*model.Media, error)
	updateFn       func(ctx context.Context, data *model.Media) error
	updateStatusFn func(ctx context.Context, id int64, expectedStatus, newStatus int64) (sql.Result, error)
	insertFn       func(ctx context.Context, data *model.Media) (sql.Result, error)
	deleteFn       func(ctx context.Context, id int64) error
	delCacheFn     func(ctx context.Context, id int64) error
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

func (f *fakeMediaModel) DelCache(ctx context.Context, id int64) error {
	if f.delCacheFn == nil {
		panic("fakeMediaModel: DelCache not configured")
	}
	return f.delCacheFn(ctx, id)
}

// fakeMediaCommandModel 提供 model.MediaCommandModel 的可注入替身；未设置的方法
// 调用会 panic，便于测试暴露调用了预期之外的方法。
type fakeMediaCommandModel struct {
	createMediaFn          func(ctx context.Context, media *model.Media, idem idempotencyx.IdempotencyRecord) (model.MediaCommandResult, error)
	softDeleteFn           func(ctx context.Context, mediaID int64, event outboxx.Event) error
	enqueueObjectCleanupFn func(ctx context.Context, event outboxx.Event) error
}

func (f *fakeMediaCommandModel) EnqueueObjectCleanup(ctx context.Context, event outboxx.Event) error {
	if f.enqueueObjectCleanupFn == nil {
		return nil
	}
	return f.enqueueObjectCleanupFn(ctx, event)
}

func (f *fakeMediaCommandModel) CreateMedia(ctx context.Context, media *model.Media, idem idempotencyx.IdempotencyRecord) (model.MediaCommandResult, error) {
	if f.createMediaFn == nil {
		panic("fakeMediaCommandModel: CreateMedia not configured")
	}
	return f.createMediaFn(ctx, media, idem)
}

func (f *fakeMediaCommandModel) SoftDelete(ctx context.Context, mediaID int64, event outboxx.Event) error {
	if f.softDeleteFn == nil {
		panic("fakeMediaCommandModel: SoftDelete not configured")
	}
	return f.softDeleteFn(ctx, mediaID, event)
}
