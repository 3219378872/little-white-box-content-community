package cursor

import (
	"errors"
	"testing"
	"time"
)

const codecTestSecret = "0123456789abcdef0123456789abcdef"

func TestCodecValidatesSignatureExpiryAndBinding(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	codec, err := New(codecTestSecret, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	binding := Binding{
		IdentityHash: IdentityHash("u:42"), RequestID: "request-1", Scene: "home",
		SessionID: "session-1", ExperimentID: "experiment-a", PageSize: 20,
	}
	token, err := codec.Encode("snapshot-1", 20, now.Add(time.Minute).Unix(), binding)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	payload, err := codec.Decode(token, binding)
	if err != nil || payload.SnapshotID != "snapshot-1" || payload.Offset != 20 {
		t.Fatalf("Decode() payload = %+v, error = %v", payload, err)
	}

	tampered := token[:len(token)-1] + "A"
	if _, err := codec.Decode(tampered, binding); !errors.Is(err, ErrInvalid) {
		t.Fatalf("tampered cursor error = %v, want ErrInvalid", err)
	}
	mismatched := binding
	mismatched.RequestID = "request-2"
	if _, err := codec.Decode(token, mismatched); !errors.Is(err, ErrMismatch) {
		t.Fatalf("mismatched cursor error = %v, want ErrMismatch", err)
	}
	expiredCodec, err := New(codecTestSecret, func() time.Time { return now.Add(2 * time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := expiredCodec.Decode(token, binding); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired cursor error = %v, want ErrExpired", err)
	}
}

func TestCodecRequiresStrongSecret(t *testing.T) {
	if _, err := New("short", time.Now); err == nil {
		t.Fatal("New() accepted a short cursor secret")
	}
}
