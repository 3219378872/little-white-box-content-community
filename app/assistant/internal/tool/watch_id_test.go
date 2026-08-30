package tool

import "testing"

func TestNormalizeWatchPostIDOnlyCorrectsToOwnHit(t *testing.T) {
	session := &Session{Source: "watch", WatchPostIDs: []int64{352522763304570880}}
	if got := normalizeWatchPostID(session, 352522763304570900); got != 352522763304570880 {
		t.Fatalf("rounded id=%d", got)
	}
	if got := normalizeWatchPostID(session, 352522763304571905); got != 352522763304571905 {
		t.Fatalf("unrelated id was corrected: %d", got)
	}
}

func TestNormalizeWatchPostIDDoesNotRelaxOtherRuns(t *testing.T) {
	session := &Session{Source: "user", WatchPostIDs: []int64{100}}
	if got := normalizeWatchPostID(session, 101); got != 101 {
		t.Fatalf("non-watch run id=%d", got)
	}
}
