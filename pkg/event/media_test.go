package event

import (
	"encoding/json"
	"testing"
)

func TestMediaDeletedEventValidate(t *testing.T) {
	if err := (MediaDeletedEvent{}).Validate(); err == nil {
		t.Fatal("expected validation error for empty event")
	}
	e := MediaDeletedEvent{MediaID: 1, S3ObjectKey: "a/b.jpg", Bucket: "xbh-media", DeletedAt: 1}
	if err := e.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestMediaDeletedEventRejectsEmptyObjectKey(t *testing.T) {
	if err := (MediaDeletedEvent{MediaID: 1}).Validate(); err == nil {
		t.Fatal("expected validation error for empty s3_object_key")
	}
}

func TestMediaDeletedEventAllowsUploadCompensationWithoutMediaRow(t *testing.T) {
	e := MediaDeletedEvent{S3ObjectKey: "orphan/object.jpg", Reason: "upload_compensation"}
	if err := e.Validate(); err != nil {
		t.Fatalf("upload compensation event rejected: %v", err)
	}
}

func TestMediaDeletedEventPayloadContract(t *testing.T) {
	// 消费端（media/mq）解析的字段名必须与此契约一致。
	e := MediaDeletedEvent{MediaID: 7, S3ObjectKey: "k/v.jpg", Bucket: "b", DeletedAt: 123}
	body, err := e.MarshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"media_id":      float64(7),
		"s3_object_key": "k/v.jpg",
		"bucket":        "b",
		"deleted_at":    float64(123),
	} {
		if decoded[key] != want {
			t.Fatalf("payload field %s = %v, want %v", key, decoded[key], want)
		}
	}
}
