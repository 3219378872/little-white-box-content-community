package logic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"esx/app/media/rpc/internal/config"
	model2 "esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/storage"
	"esx/app/media/rpc/internal/svc"
	"esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"esx/pkg/mqx"
	"esx/pkg/outboxx"
	"strconv"
	"testing"

	"esx/pkg/errx"
)

func newDeleteLogicWithFake(f *fakeMediaModel, cmd *fakeMediaCommandModel) *DeleteMediaLogic {
	return NewDeleteMediaLogic(context.Background(), &svc.ServiceContext{
		MediaModel:        f,
		MediaCommandModel: cmd,
		Config: config.Config{
			S3Storage: storage.Config{Bucket: "xbh-media-test"},
		},
	})
}

func TestDeleteMedia_RejectsNonPositiveIds(t *testing.T) {
	l := newDeleteLogicWithFake(&fakeMediaModel{}, &fakeMediaCommandModel{})

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
		findOneFn: func(_ context.Context, _ int64) (*model2.Media, error) {
			return nil, model2.ErrNotFound
		},
	}
	l := newDeleteLogicWithFake(f, &fakeMediaCommandModel{})

	_, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: 1, UserId: 1})
	if code := errx.GetCode(err); code != errx.MediaNotFound {
		t.Fatalf("expected MediaNotFound, got code=%d err=%v", code, err)
	}
}

func TestDeleteMedia_DBErrorOnFindMapsToSystemError(t *testing.T) {
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model2.Media, error) {
			return nil, errors.New("boom")
		},
	}
	l := newDeleteLogicWithFake(f, &fakeMediaCommandModel{})

	_, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: 1, UserId: 1})
	if code := errx.GetCode(err); code != errx.SystemError {
		t.Fatalf("expected SystemError, got code=%d err=%v", code, err)
	}
}

func TestDeleteMedia_IdempotentWhenAlreadyDeleted(t *testing.T) {
	// status=0 直接返回成功，不校验归属，不写库、不投递事件。
	softDeleteCalled := false
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model2.Media, error) {
			return &model2.Media{Id: 1, UserId: 999, Status: 0}, nil
		},
	}
	cmd := &fakeMediaCommandModel{
		softDeleteFn: func(_ context.Context, _ int64, _ []outboxx.Event) error {
			softDeleteCalled = true
			return nil
		},
	}
	l := newDeleteLogicWithFake(f, cmd)

	// 注意调用方 UserId=1，但资源归属 UserId=999；已删状态下不应触发权限错误。
	resp, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: 1, UserId: 1})
	if err != nil {
		t.Fatalf("expected idempotent success, got err=%v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil DeleteMediaResp")
	}
	if softDeleteCalled {
		t.Fatal("SoftDelete must not be called when row is already soft-deleted")
	}
}

func TestDeleteMedia_RejectsNonOwner(t *testing.T) {
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model2.Media, error) {
			return &model2.Media{Id: 1, UserId: 999, Status: 1}, nil
		},
	}
	cmd := &fakeMediaCommandModel{
		softDeleteFn: func(_ context.Context, _ int64, _ []outboxx.Event) error {
			t.Fatal("SoftDelete must not run for non-owner")
			return nil
		},
	}
	l := newDeleteLogicWithFake(f, cmd)

	_, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: 1, UserId: 1})
	if code := errx.GetCode(err); code != errx.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got code=%d err=%v", code, err)
	}
}

func TestDeleteMedia_OwnerSoftDeletesWithOutboxEvent(t *testing.T) {
	stored := &model2.Media{Id: 1, UserId: 7, Status: 1, ObjectKey: sql.NullString{String: "obj/key", Valid: true}}
	var deletedID int64
	var enqueued outboxx.Event
	cacheDeleted := false
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model2.Media, error) {
			return stored, nil
		},
		delCacheFn: func(_ context.Context, id int64) error {
			cacheDeleted = true
			return nil
		},
	}
	cmd := &fakeMediaCommandModel{
		softDeleteFn: func(_ context.Context, mediaID int64, events []outboxx.Event) error {
			deletedID = mediaID
			if len(events) > 0 {
				enqueued = events[0]
			}
			return nil
		},
	}
	l := newDeleteLogicWithFake(f, cmd)

	_, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: 1, UserId: 7})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !cacheDeleted {
		t.Fatal("expected DelCache after successful soft delete")
	}
	if deletedID != 1 {
		t.Fatalf("expected SoftDelete called with id=1, got %d", deletedID)
	}
	if enqueued.Topic != mqx.TopicMediaDelete {
		t.Fatalf("expected topic %s, got %s", mqx.TopicMediaDelete, enqueued.Topic)
	}
	if enqueued.Key == "" || enqueued.ID <= 0 {
		t.Fatal("expected outbox event id/key set")
	}
	var payload map[string]any
	if err := json.Unmarshal(enqueued.Payload, &payload); err != nil {
		t.Fatalf("payload not json: %v", err)
	}
	if payload["media_id"] != float64(1) {
		t.Fatalf("expected payload media_id=1, got %v", payload["media_id"])
	}
	if payload["s3_object_key"] != "obj/key" {
		t.Fatalf("expected payload s3_object_key=obj/key, got %v", payload["s3_object_key"])
	}
	if payload["bucket"] != "xbh-media-test" {
		t.Fatalf("expected payload bucket=xbh-media-test, got %v", payload["bucket"])
	}
}

func TestDeleteMedia_NoObjectKeySkipsEventButSoftDeletes(t *testing.T) {
	// 无 S3 对象的媒体行仍可删除：仅软删，不投递 outbox 事件。
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model2.Media, error) {
			return &model2.Media{Id: 1, UserId: 7, Status: 1}, nil
		},
		delCacheFn: func(_ context.Context, _ int64) error {
			return nil
		},
	}
	var enqueued outboxx.Event
	cmd := &fakeMediaCommandModel{
		softDeleteFn: func(_ context.Context, _ int64, events []outboxx.Event) error {
			if len(events) > 0 {
				enqueued = events[0]
			}
			return nil
		},
	}
	l := newDeleteLogicWithFake(f, cmd)

	_, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: 1, UserId: 7})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if enqueued.ID != 0 {
		t.Fatalf("expected zero event when no object key, got %+v", enqueued)
	}
}

func TestDeleteMedia_CommandModelErrorMapsToSystemError(t *testing.T) {
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model2.Media, error) {
			return &model2.Media{Id: 1, UserId: 7, Status: 1, ObjectKey: sql.NullString{String: "obj/key", Valid: true}}, nil
		},
	}
	cmd := &fakeMediaCommandModel{
		softDeleteFn: func(_ context.Context, _ int64, _ []outboxx.Event) error {
			return errors.New("conn lost")
		},
	}
	l := newDeleteLogicWithFake(f, cmd)

	_, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: 1, UserId: 7})
	if code := errx.GetCode(err); code != errx.SystemError {
		t.Fatalf("expected SystemError, got code=%d err=%v", code, err)
	}
}

func TestDeleteMedia_EnqueuesThumbnailObject(t *testing.T) {
	stored := &model2.Media{
		Id: 1, UserId: 7, Status: 1,
		ObjectKey:          sql.NullString{String: "original/202609/a.jpg", Valid: true},
		ThumbnailObjectKey: sql.NullString{String: "thumb/202609/b.jpg", Valid: true},
	}
	var keys []string
	f := &fakeMediaModel{
		findOneFn:  func(_ context.Context, _ int64) (*model2.Media, error) { return stored, nil },
		delCacheFn: func(_ context.Context, _ int64) error { return nil },
	}
	cmd := &fakeMediaCommandModel{
		softDeleteFn: func(_ context.Context, _ int64, events []outboxx.Event) error {
			for _, event := range events {
				var payload map[string]any
				if err := json.Unmarshal(event.Payload, &payload); err != nil {
					t.Fatalf("payload: %v", err)
				}
				key, _ := payload["s3_object_key"].(string)
				keys = append(keys, key)
			}
			return nil
		},
	}
	if _, err := newDeleteLogicWithFake(f, cmd).DeleteMedia(&pb.DeleteMediaReq{MediaId: 1, UserId: 7}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(keys) != 2 || keys[0] != "original/202609/a.jpg" || keys[1] != "thumb/202609/b.jpg" {
		t.Fatalf("expected original and thumb keys, got %v", keys)
	}
}

func TestDeleteMedia_DelCacheFailureStillSucceeds(t *testing.T) {
	f := &fakeMediaModel{
		findOneFn: func(_ context.Context, _ int64) (*model2.Media, error) {
			return &model2.Media{Id: 1, UserId: 7, Status: 1, ObjectKey: sql.NullString{String: "obj/key", Valid: true}}, nil
		},
		delCacheFn: func(_ context.Context, _ int64) error {
			return errors.New("redis down")
		},
	}
	cmd := &fakeMediaCommandModel{
		softDeleteFn: func(_ context.Context, _ int64, _ []outboxx.Event) error { return nil },
	}
	if _, err := newDeleteLogicWithFake(f, cmd).DeleteMedia(&pb.DeleteMediaReq{MediaId: 1, UserId: 7}); err != nil {
		t.Fatalf("CORE-053: cache invalidation must not fail the delete, got %v", err)
	}
}

func TestBuildMediaDeletedOutboxEvent(t *testing.T) {
	event, err := buildMediaDeletedOutboxEvent(42, "k/obj.jpg", "xbh-media")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if event.Topic != mqx.TopicMediaDelete || event.Tag != mqx.TagDefault {
		t.Fatalf("unexpected topic/tag: %s/%s", event.Topic, event.Tag)
	}
	if event.Key != strconv.FormatInt(event.ID, 10) {
		t.Fatal("expected message key to equal event id")
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["media_id"] != float64(42) || payload["s3_object_key"] != "k/obj.jpg" || payload["bucket"] != "xbh-media" {
		t.Fatalf("unexpected payload: %v", payload)
	}
}
