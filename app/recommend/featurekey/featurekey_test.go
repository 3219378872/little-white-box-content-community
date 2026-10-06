package featurekey

import "testing"

// 键名是 recommend-mq 与 recommend-rpc 之间的存储契约，这里用固定值锁住逐字节格式。
func TestKeysKeepStoredFormat(t *testing.T) {
	space := New("v2")
	cases := []struct{ got, want string }{
		{space.Viewer("u:42"), "feature:v2:u:42"},
		{space.ViewerRecent("u:42"), "feature:v2:u:42:recent"},
		{space.ViewerNegative("u:42"), "feature:v2:u:42:negative"},
		{space.LoggedInViewerPattern(), "feature:v2:u:*"},
		{space.Dedup("evt-1"), "feature:v2:dedup:evt-1"},
		{space.Post(1001), "feature:v2:post:1001"},
		{space.User(7), "feature:v2:user:7"},
		{OptOutKey(7), "personalization:optout:7"},
		{Identity(42, "ignored-device"), "u:42"},
		{Identity(0, ""), ""},
		{Identity(0, "private-device-id"), "a:68c3e01c181d47b8"},
		{Identity(-1, "private-device-id"), "a:68c3e01c181d47b8"},
		{New("v3").ViewerRecent("a:abcdef0"), "feature:v3:a:abcdef0:recent"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("key = %q, want %q", tc.got, tc.want)
		}
	}
}

func TestIdentityDoesNotExposeAnonymousIdentifier(t *testing.T) {
	key := Identity(0, "private-device-id")
	if key == "" || key == "a:private-device-id" {
		t.Fatalf("anonymous identity was not hashed: %q", key)
	}
	if key != Identity(0, "private-device-id") {
		t.Fatal("anonymous identity hashing is not deterministic")
	}
}
