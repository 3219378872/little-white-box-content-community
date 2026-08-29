package runtime

import (
	"strings"
	"testing"

	"esx/app/assistant/internal/store"
)

func TestSelectKeepLastTwentyPercent(t *testing.T) {
	msgs := make([]store.Message, 0, 10)
	for i := 0; i < 10; i++ {
		msgs = append(msgs, store.Message{ID: int64(i + 1), Role: store.RoleUser, Content: strings.Repeat("字", 40), Visible: true})
	}
	total := EstimateMessageTokens(msgs)
	keep := total / 5
	selected := SelectKeep(msgs, keep, nil)
	if len(selected) == 0 {
		t.Fatal("expected some kept messages")
	}
	if selected[len(selected)-1].ID != 10 {
		t.Fatalf("should keep the newest, got %+v", selected)
	}
	if EstimateMessageTokens(selected) > keep+EstimateTokens(strings.Repeat("字", 40)) {
		t.Fatalf("kept too many tokens: %d vs %d", EstimateMessageTokens(selected), keep)
	}
}
