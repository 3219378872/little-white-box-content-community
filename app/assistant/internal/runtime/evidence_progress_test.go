package runtime

import "testing"

func TestEvidenceProgressIgnoresVolatileHandles(t *testing.T) {
	first := encodeToolResultJSONWithChanges(`{"sources":[{"handle":"one","kind":"post","title":"title","retrieved_evidence":[{"id":"a","handle":"one","text":"same","kind":"post","retrievedAtMs":1}]}]}`, nil, nil)
	second := encodeToolResultJSONWithChanges(`{"sources":[{"handle":"two","kind":"post","title":"title","retrieved_evidence":[{"id":"b","handle":"two","text":"same","kind":"post","retrievedAtMs":2}]}]}`, nil, nil)
	if normalizeEvidenceResult(first) != normalizeEvidenceResult(second) {
		t.Fatal("volatile source metadata was treated as progress")
	}
	if normalizeEvidenceResult(first) == normalizeEvidenceResult(encodeToolResultJSONWithChanges("different result", nil, nil)) {
		t.Fatal("new information was discarded")
	}
}
