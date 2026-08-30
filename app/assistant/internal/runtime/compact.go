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
	live := liveMessages(messages)
	total := EstimateMessageTokens(live)
	if total < windowTokens/2 {
		return false
	}
	// A single oversized message (or a set of mandatory tool messages) cannot
	// be reduced by another compact pass. Requiring at least one droppable
	// message prevents the new prompt epoch from compacting forever.
	selected := SelectKeep(live, maxInt(total/5, 1), unfinishedCallIDs(live))
	return len(selected) < len(live)
}

func EstimateMessageTokens(messages []store.Message) int {
	total := 0
	for _, msg := range messages {
		if msg.DeletedAtMs != 0 || msg.Compacted {
			continue
		}
		total += estimateStoredTokens(msg)
	}
	return total
}

func estimateStoredTokens(msg store.Message) int {
	if len(msg.APIContent) > 0 {
		return EstimateTokens(string(msg.APIContent))
	}
	return EstimateTokens(msg.Content)
}

func SelectKeep(messages []store.Message, keepTokens int, unfinished map[string]struct{}) []store.Message {
	if keepTokens <= 0 {
		keepTokens = 1
	}
	kept := make([]store.Message, 0)
	used := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.DeletedAtMs != 0 || msg.Compacted {
			continue
		}
		force := messageHasUnfinishedCall(msg, unfinished)
		cost := estimateStoredTokens(msg)
		if !force && used+cost > keepTokens && len(kept) > 0 {
			continue
		}
		kept = append([]store.Message{msg}, kept...)
		used += cost
	}
	return kept
}

func unfinishedCallIDs(messages []store.Message) map[string]struct{} {
	calls := unmatchedToolCalls(HistoryTurns(messages))
	out := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		if call.ID != "" {
			out[call.ID] = struct{}{}
		}
	}
	return out
}

func messageHasUnfinishedCall(msg store.Message, unfinished map[string]struct{}) bool {
	if len(unfinished) == 0 {
		return false
	}
	turn, ok := prompt.DecodeTurn(msg.APIContent)
	if !ok {
		return false
	}
	if _, ok := unfinished[turn.ToolCallID]; ok && turn.ToolCallID != "" {
		return true
	}
	for _, call := range turn.ToolCalls {
		if _, ok := unfinished[call.ID]; ok {
			return true
		}
	}
	return false
}

func HistoryTurns(messages []store.Message) []prompt.Turn {
	out := make([]prompt.Turn, 0, len(messages))
	for _, msg := range messages {
		if msg.DeletedAtMs != 0 || msg.Compacted {
			continue
		}
		if turn, ok := turnFromMessage(msg); ok {
			out = append(out, turn)
		}
	}
	return out
}

func liveMessages(messages []store.Message) []store.Message {
	out := make([]store.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.DeletedAtMs == 0 && !msg.Compacted {
			out = append(out, msg)
		}
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func turnFromMessage(msg store.Message) (prompt.Turn, bool) {
	toolish := msg.Role == store.RoleTool || msg.Kind == store.KindTool
	if !msg.Visible && !toolish {
		return prompt.Turn{}, false
	}
	if turn, ok := prompt.DecodeTurn(msg.APIContent); ok {
		if turn.Role == "" {
			turn.Role = msg.Role
		}
		return turn, true
	}
	switch msg.Role {
	case store.RoleUser, store.RoleAssistant, store.RoleTool:
		return prompt.Turn{Role: msg.Role, Content: msg.Content}, true
	default:
		return prompt.Turn{}, false
	}
}
