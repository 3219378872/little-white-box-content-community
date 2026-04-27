package logic

import (
	"context"
	"errors"
	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/svc"
	"esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"testing"

	"errx"
)

func newBatchLogicWithFake(f *fakeMediaModel) *BatchGetMediaLogic {
	return NewBatchGetMediaLogic(context.Background(), &svc.ServiceContext{MediaModel: f})
}

func TestBatchGetMedia_RejectsEmptyIds(t *testing.T) {
	l := newBatchLogicWithFake(&fakeMediaModel{})

	_, err := l.BatchGetMedia(&pb.BatchGetMediaReq{MediaIds: nil})
	if code := errx.GetCode(err); code != errx.ParamError {
		t.Fatalf("nil ids: expected ParamError, got code=%d err=%v", code, err)
	}

	_, err = l.BatchGetMedia(&pb.BatchGetMediaReq{MediaIds: []int64{}})
	if code := errx.GetCode(err); code != errx.ParamError {
		t.Fatalf("empty slice: expected ParamError, got code=%d err=%v", code, err)
	}
}

func TestBatchGetMedia_RejectsOversizedBatch(t *testing.T) {
	ids := make([]int64, 101)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	l := newBatchLogicWithFake(&fakeMediaModel{})

	_, err := l.BatchGetMedia(&pb.BatchGetMediaReq{MediaIds: ids})
	if code := errx.GetCode(err); code != errx.ParamError {
		t.Fatalf("expected ParamError for size=101, got code=%d err=%v", code, err)
	}
}

func TestBatchGetMedia_AcceptsExactly100(t *testing.T) {
	ids := make([]int64, 100)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	called := false
	f := &fakeMediaModel{
		findByIdsFn: func(_ context.Context, got []int64) ([]*model.Media, error) {
			called = true
			if len(got) != 100 {
				t.Fatalf("expected 100 ids passed through, got %d", len(got))
			}
			return nil, nil
		},
	}
	l := newBatchLogicWithFake(f)

	if _, err := l.BatchGetMedia(&pb.BatchGetMediaReq{MediaIds: ids}); err != nil {
		t.Fatalf("unexpected err for size=100: %v", err)
	}
	if !called {
		t.Fatal("FindByIds was not invoked for size=100 batch")
	}
}

func TestBatchGetMedia_RejectsNonPositiveIdInList(t *testing.T) {
	l := newBatchLogicWithFake(&fakeMediaModel{})

	cases := [][]int64{{0}, {-1}, {1, 0, 2}, {1, -5}}
	for _, ids := range cases {
		_, err := l.BatchGetMedia(&pb.BatchGetMediaReq{MediaIds: ids})
		if code := errx.GetCode(err); code != errx.ParamError {
			t.Fatalf("ids=%v: expected ParamError, got code=%d err=%v", ids, code, err)
		}
	}
}

func TestBatchGetMedia_FiltersNonNormalStatus(t *testing.T) {
	rows := []*model.Media{
		{Id: 1, FileName: "a", FileType: "image", Url: "u1", Status: 1},
		{Id: 2, FileName: "b", FileType: "image", Url: "u2", Status: 0}, // 软删
		{Id: 3, FileName: "c", FileType: "image", Url: "u3", Status: 2}, // 处理中
		{Id: 4, FileName: "d", FileType: "image", Url: "u4", Status: 1},
	}
	f := &fakeMediaModel{
		findByIdsFn: func(_ context.Context, _ []int64) ([]*model.Media, error) {
			return rows, nil
		},
	}
	l := newBatchLogicWithFake(f)

	resp, err := l.BatchGetMedia(&pb.BatchGetMediaReq{MediaIds: []int64{1, 2, 3, 4}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(resp.Medias) != 2 {
		t.Fatalf("expected 2 visible rows after filtering, got %d: %+v", len(resp.Medias), resp.Medias)
	}
	gotIds := map[int64]bool{}
	for _, m := range resp.Medias {
		gotIds[m.Id] = true
	}
	if !gotIds[1] || !gotIds[4] {
		t.Fatalf("expected ids {1,4} to remain, got %+v", gotIds)
	}
	if gotIds[2] || gotIds[3] {
		t.Fatalf("soft-deleted/processing rows leaked: %+v", gotIds)
	}
}

func TestBatchGetMedia_DBErrorMapsToSystemError(t *testing.T) {
	f := &fakeMediaModel{
		findByIdsFn: func(_ context.Context, _ []int64) ([]*model.Media, error) {
			return nil, errors.New("db down")
		},
	}
	l := newBatchLogicWithFake(f)

	_, err := l.BatchGetMedia(&pb.BatchGetMediaReq{MediaIds: []int64{1}})
	if code := errx.GetCode(err); code != errx.SystemError {
		t.Fatalf("expected SystemError, got code=%d err=%v", code, err)
	}
}

func TestBatchGetMedia_EmptyRowsReturnsEmptyList(t *testing.T) {
	f := &fakeMediaModel{
		findByIdsFn: func(_ context.Context, _ []int64) ([]*model.Media, error) {
			return nil, nil
		},
	}
	l := newBatchLogicWithFake(f)

	resp, err := l.BatchGetMedia(&pb.BatchGetMediaReq{MediaIds: []int64{99}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp == nil || resp.Medias == nil {
		t.Fatalf("expected non-nil resp and empty Medias slice, got resp=%v", resp)
	}
	if len(resp.Medias) != 0 {
		t.Fatalf("expected empty Medias, got %d entries", len(resp.Medias))
	}
}
