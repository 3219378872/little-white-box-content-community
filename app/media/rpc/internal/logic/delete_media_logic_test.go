package logic

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"errx"

	"esx/app/media/internal/model"
	"esx/app/media/internal/svc"
	"esx/app/media/pb/xiaobaihe/media/pb"
)

func newDeleteLogicWithFake(f *fakeMediaModel) *DeleteMediaLogic {
	return NewDeleteMediaLogic(context.Background(), &svc.ServiceContext{MediaModel: f})
}

func TestDeleteMedia_RejectsNonPositiveIds(t *testing.T) {
	l := newDeleteLogicWithFake(&fakeMediaModel{})

	cases := []*pb.DeleteMediaReq{
		{MediaId: 0, UserId: 1},
		{MediaId: -1, UserId: 1},
		{MediaId: 1, UserId: 0},
		{MediaId: 1, UserId: -1},
	}
	for _, req := range cases {
		_, err := l.DeleteMedia(req)
		if code := errx.GetCode(err); code != errx.ParamError {
			t.Fatalf("%+v: expected ParamError, got code=%d err=%v", req, code, err)
		}
	}
}

func TestDeleteMedia_NotFoundMapsToMediaNotFound(t *testing.T) {
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model.Media, error) {
			return nil, model.ErrNotFound
		},
	}
	l := newDeleteLogicWithFake(f)

	_, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: 1, UserId: 1})
	if code := errx.GetCode(err); code != errx.MediaNotFound {
		t.Fatalf("expected MediaNotFound, got code=%d err=%v", code, err)
	}
}

func TestDeleteMedia_DBErrorOnFindMapsToSystemError(t *testing.T) {
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model.Media, error) {
			return nil, errors.New("boom")
		},
	}
	l := newDeleteLogicWithFake(f)

	_, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: 1, UserId: 1})
	if code := errx.GetCode(err); code != errx.SystemError {
		t.Fatalf("expected SystemError, got code=%d err=%v", code, err)
	}
}

func TestDeleteMedia_IdempotentWhenAlreadyDeleted(t *testing.T) {
	// status=0 直接返回成功，不校验归属，不写库。
	updateStatusCalled := false
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model.Media, error) {
			return &model.Media{Id: 1, UserId: 999, Status: 0}, nil
		},
		updateStatusFn: func(_ context.Context, _ int64, _, _ int64) (sql.Result, error) {
			updateStatusCalled = true
			return nil, nil
		},
	}
	l := newDeleteLogicWithFake(f)

	// 注意调用方 UserId=1，但资源归属 UserId=999；已删状态下不应触发权限错误。
	resp, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: 1, UserId: 1})
	if err != nil {
		t.Fatalf("expected idempotent success, got err=%v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil DeleteMediaResp")
	}
	if updateStatusCalled {
		t.Fatal("UpdateStatus must not be called when row is already soft-deleted")
	}
}

func TestDeleteMedia_RejectsNonOwner(t *testing.T) {
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model.Media, error) {
			return &model.Media{Id: 1, UserId: 999, Status: 1}, nil
		},
		updateStatusFn: func(_ context.Context, _ int64, _, _ int64) (sql.Result, error) {
			t.Fatal("UpdateStatus must not run for non-owner")
			return nil, nil
		},
	}
	l := newDeleteLogicWithFake(f)

	_, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: 1, UserId: 1})
	if code := errx.GetCode(err); code != errx.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got code=%d err=%v", code, err)
	}
}

func TestDeleteMedia_OwnerSoftDeletesAndPersistsStatusZero(t *testing.T) {
	stored := &model.Media{Id: 1, UserId: 7, Status: 1}
	var updatedId int64
	var updatedExpectedStatus, updatedNewStatus int64
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model.Media, error) {
			return stored, nil
		},
		updateStatusFn: func(_ context.Context, id int64, expectedStatus, newStatus int64) (sql.Result, error) {
			updatedId = id
			updatedExpectedStatus = expectedStatus
			updatedNewStatus = newStatus
			return &fakeResult{rowsAffected: 1}, nil
		},
	}
	l := newDeleteLogicWithFake(f)

	_, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: 1, UserId: 7})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if updatedId != 1 {
		t.Fatalf("expected UpdateStatus called with id=1, got %d", updatedId)
	}
	if updatedExpectedStatus != 1 {
		t.Fatalf("expected expectedStatus=1, got %d", updatedExpectedStatus)
	}
	if updatedNewStatus != 0 {
		t.Fatalf("expected newStatus=0, got %d", updatedNewStatus)
	}
}

func TestDeleteMedia_DBErrorOnUpdateMapsToSystemError(t *testing.T) {
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model.Media, error) {
			return &model.Media{Id: 1, UserId: 7, Status: 1}, nil
		},
		updateStatusFn: func(_ context.Context, _ int64, _, _ int64) (sql.Result, error) {
			return nil, errors.New("conn lost")
		},
	}
	l := newDeleteLogicWithFake(f)

	_, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: 1, UserId: 7})
	if code := errx.GetCode(err); code != errx.SystemError {
		t.Fatalf("expected SystemError, got code=%d err=%v", code, err)
	}
}
