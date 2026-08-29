package runtime

import (
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"unicode/utf8"
)

func EstimateTokens(text string) int {
	n := utf8.RuneCountInString(text)
	if n <= 0 {
		return 0
	}
	est := (n + 3) / 4
	if est < 1 {
		return 1
	}
	return est
}

func ShouldCompact(messages []store.Message, windowTokens int) bool {
	if windowTokens <= 0 {
		windowTokens = 128000
	}
	return EstimateMessageTokens(messages) >= windowTokens/2
}

func EstimateMessageTokens(messages []store.Message) int {
	total := 0
	for _, msg := range messages {
		if msg.DeletedAtMs != 0 {
			continue
		}
		total += EstimateTokens(msg.Content)
	}
	return total
}

func SelectKeep(messages []store.Message, keepTokens int, unfinished map[string]struct{}) []store.Message {
	if keepTokens <= 0 {
		keepTokens = 1
	}
	kept := make([]store.Message, 0)
	used := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.DeletedAtMs != 0 {
			continue
		}
		_, marked := unfinished[msg.Kind]
		force := marked || msg.Kind == "tool" || msg.Role == store.RoleTool
		cost := EstimateTokens(msg.Content)
		if !force && used+cost > keepTokens && len(kept) > 0 {
			continue
		}
		kept = append([]store.Message{msg}, kept...)
		used += cost
	}
	return kept
}

func HistoryTurns(messages []store.Message) []prompt.Turn {
	out := make([]prompt.Turn, 0, len(messages))
	for _, msg := range messages {
		if msg.DeletedAtMs != 0 || !msg.Visible {
			continue
		}
		if msg.Role == store.RoleUser || msg.Role == store.RoleAssistant {
			out = append(out, prompt.Turn{Role: msg.Role, Content: msg.Content})
		}
	}
	return out
}
