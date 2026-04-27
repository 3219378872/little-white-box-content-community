package logic

import (
	"context"
	"database/sql"
	"errors"
	model2 "esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/svc"
	"esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"testing"
	"time"

	"errx"
)

func newGetLogicWithFake(f *fakeMediaModel) *GetMediaLogic {
	return NewGetMediaLogic(context.Background(), &svc.ServiceContext{MediaModel: f})
}

func TestGetMedia_RejectsNonPositiveId(t *testing.T) {
	l := newGetLogicWithFake(&fakeMediaModel{})

	for _, id := range []int64{0, -1} {
		_, err := l.GetMedia(&pb.GetMediaReq{MediaId: id})
		if code := errx.GetCode(err); code != errx.ParamError {
			t.Fatalf("id=%d: expected ParamError(%d), got code=%d err=%v",
				id, errx.ParamError, code, err)
		}
	}
}

func TestGetMedia_NotFoundMapsToMediaNotFound(t *testing.T) {
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model2.Media, error) {
			return nil, model2.ErrNotFound
		},
	}
	l := newGetLogicWithFake(f)

	_, err := l.GetMedia(&pb.GetMediaReq{MediaId: 42})
	if code := errx.GetCode(err); code != errx.MediaNotFound {
		t.Fatalf("expected MediaNotFound(%d), got code=%d err=%v",
			errx.MediaNotFound, code, err)
	}
}

func TestGetMedia_DBErrorMapsToSystemError(t *testing.T) {
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model2.Media, error) {
			return nil, errors.New("db exploded")
		},
	}
	l := newGetLogicWithFake(f)

	_, err := l.GetMedia(&pb.GetMediaReq{MediaId: 7})
	if code := errx.GetCode(err); code != errx.SystemError {
		t.Fatalf("expected SystemError(%d), got code=%d err=%v",
			errx.SystemError, code, err)
	}
}

func TestGetMedia_SoftDeletedRowMapsToMediaNotFound(t *testing.T) {
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model2.Media, error) {
			return &model2.Media{Id: 7, Status: 0}, nil
		},
	}
	l := newGetLogicWithFake(f)

	_, err := l.GetMedia(&pb.GetMediaReq{MediaId: 7})
	if code := errx.GetCode(err); code != errx.MediaNotFound {
		t.Fatalf("expected MediaNotFound(%d), got code=%d err=%v",
			errx.MediaNotFound, code, err)
	}
}

func TestGetMedia_ProcessingStatusMapsToMediaNotFound(t *testing.T) {
	// status=2 处理中也应被视为不可见。
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model2.Media, error) {
			return &model2.Media{Id: 7, Status: 2}, nil
		},
	}
	l := newGetLogicWithFake(f)

	_, err := l.GetMedia(&pb.GetMediaReq{MediaId: 7})
	if code := errx.GetCode(err); code != errx.MediaNotFound {
		t.Fatalf("expected MediaNotFound for status=2, got code=%d err=%v", code, err)
	}
}

func TestGetMedia_HappyPathReturnsMappedInfo(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	want := &model2.Media{
		Id:           101,
		UserId:       9,
		FileName:     "a.jpg",
		FileType:     "image",
		Url:          "http://x/a.jpg",
		ThumbnailUrl: sql.NullString{Valid: true, String: "http://x/a_thumb.jpg"},
		FileSize:     1024,
		Width:        sql.NullInt64{Valid: true, Int64: 800},
		Height:       sql.NullInt64{Valid: true, Int64: 600},
		Duration:     sql.NullInt64{Valid: false},
		Status:       1,
		CreatedAt:    now,
	}
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model2.Media, error) {
			return want, nil
		},
	}
	l := newGetLogicWithFake(f)

	resp, err := l.GetMedia(&pb.GetMediaReq{MediaId: 101})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got := resp.GetMedia()
	if got == nil {
		t.Fatal("resp.Media is nil")
	}
	if got.Id != 101 || got.UserId != 9 || got.FileName != "a.jpg" ||
		got.FileType != "image" || got.Url != "http://x/a.jpg" ||
		got.ThumbnailUrl != "http://x/a_thumb.jpg" ||
		got.FileSize != 1024 || got.Width != 800 || got.Height != 600 ||
		got.Duration != 0 || got.Status != 1 ||
		got.CreatedAt != now.UnixMilli() {
		t.Fatalf("unexpected mapping: %+v", got)
	}
}

func TestToPBMediaInfo_NullableFieldsFallBackToZero(t *testing.T) {
	m := &model2.Media{
		Id:           1,
		UserId:       2,
		FileName:     "f",
		FileType:     "image",
		Url:          "u",
		ThumbnailUrl: sql.NullString{Valid: false},
		FileSize:     0,
		Width:        sql.NullInt64{Valid: false},
		Height:       sql.NullInt64{Valid: false},
		Duration:     sql.NullInt64{Valid: false},
		Status:       1,
		CreatedAt:    time.Unix(0, 0),
	}
	got := toPBMediaInfo(m)

	if got.ThumbnailUrl != "" {
		t.Fatalf("null ThumbnailUrl should map to empty string, got %q", got.ThumbnailUrl)
	}
	if got.Width != 0 || got.Height != 0 || got.Duration != 0 {
		t.Fatalf("null int fields should map to 0, got w=%d h=%d dur=%d",
			got.Width, got.Height, got.Duration)
	}
}
