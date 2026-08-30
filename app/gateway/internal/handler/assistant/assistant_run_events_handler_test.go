package assistant

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestResumeAfterSeqUsesNewestCursor(t *testing.T) {
	tests := []struct {
		query  int64
		header string
		want   int64
	}{
		{query: 8, header: "12", want: 12},
		{query: 12, header: "8", want: 12},
		{query: 7, header: "bad", want: 7},
		{query: 0, header: " 9 ", want: 9},
	}
	for _, test := range tests {
		if got := resumeAfterSeq(test.query, test.header); got != test.want {
			t.Fatalf("query=%d header=%q got=%d want=%d", test.query, test.header, got, test.want)
		}
	}
}

func TestAssistantSSEHeartbeatIsACommentWithinThirtySeconds(t *testing.T) {
	if assistantSSEHeartbeatInterval > 30*time.Second {
		t.Fatalf("heartbeat interval=%s", assistantSSEHeartbeatInterval)
	}
	recorder := httptest.NewRecorder()
	if err := writeAssistantSSEHeartbeat(recorder); err != nil {
		t.Fatal(err)
	}
	if got := recorder.Body.String(); got != ": heartbeat\n\n" {
		t.Fatalf("heartbeat=%q", got)
	}
}
