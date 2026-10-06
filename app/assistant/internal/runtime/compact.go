package runtime

import (
	"strings"

	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
)

// EstimateTokens is a cheap token estimate: about four ASCII characters or one non-ASCII rune per token.
func EstimateTokens(text string) int {
	ascii := 0
	nonASCII := 0
	for _, r := range text {
		if r <= 0x7f {
			ascii++
		} else {
			nonASCII++
		}
	}
	if ascii == 0 && nonASCII == 0 {
		return 0
	}
	est := (ascii+3)/4 + nonASCII
	if est < 1 {
		return 1
	}
	return est
}

// SummaryInput picks the newest live visible messages that fit the summary budget, in original order.
func SummaryInput(messages []store.Message, budgetTokens int) string {
	if budgetTokens <= 0 {
		return ""
	}
	selected := make([]string, 0, len(messages))
	used := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if !msg.Visible || msg.DeletedAtMs != 0 || msg.Compacted {
			continue
		}
		line := msg.Role + ": " + msg.Content + "\n"
		cost := EstimateTokens(line)
		if cost > budgetTokens || used+cost > budgetTokens {
			continue
		}
		selected = append([]string{line}, selected...)
		used += cost
	}
	return strings.Join(selected, "")
}

// ShouldCompactWithAnchor compacts once the prompt reaches half the window. The provider's last
// reported prompt size overrides the local estimate when it is larger.
func ShouldCompactWithAnchor(messages []store.Message, windowTokens int, lastPromptTokens int64) bool {
	if windowTokens <= 0 {
		windowTokens = 128000
	}
	live := liveMessages(messages)
	total := EstimateMessageTokens(live)
	if lastPromptTokens > int64(total) {
		total = int(lastPromptTokens)
	}
	if total < windowTokens/2 {
		return false
	}
	// A single oversized message (or a set of mandatory tool messages) cannot
	// be reduced by another compact pass. Requiring at least one droppable
	// message prevents the new prompt epoch from compacting forever.
	selected := SelectKeep(live, maxInt(total/5, 1), unfinishedCallIDs(live))
	return len(selected) < len(live)
}

// EstimateMessageTokens sums the estimate of messages that still enter the prompt.
func EstimateMessageTokens(messages []store.Message) int {
	total := 0
	for _, msg := range messages {
		if msg.DeletedAtMs != 0 || msg.Compacted || msg.Kind == store.KindQuestion {
			continue
		}
		total += estimateStoredTokens(msg)
	}
	return total
}

// EstimatePromptTokens estimates already assembled prompt turns in their encoded form.
func EstimatePromptTokens(turns []prompt.Turn) int {
	total := 0
	for _, turn := range turns {
		total += EstimateTokens(string(prompt.EncodeTurn(turn)))
	}
	return total
}

// estimateStoredTokens prefers the provider-format payload, which is what is actually sent.
func estimateStoredTokens(msg store.Message) int {
	if len(msg.APIContent) > 0 {
		return EstimateTokens(string(msg.APIContent))
	}
	return EstimateTokens(msg.Content)
}

// SelectKeep keeps the newest messages within keepTokens, always keeping the newest one and any
// message that belongs to an unfinished tool call so the transcript stays valid.
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

// unfinishedCallIDs lists tool calls that have no result yet.
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

// messageHasUnfinishedCall reports whether a message issues or answers one of the unfinished calls.
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

// HistoryTurns converts stored messages to prompt turns, skipping deleted, compacted and question rows.
func HistoryTurns(messages []store.Message) []prompt.Turn {
	out := make([]prompt.Turn, 0, len(messages))
	for _, msg := range messages {
		if msg.DeletedAtMs != 0 || msg.Compacted || msg.Kind == store.KindQuestion {
			continue
		}
		if turn, ok := turnFromMessage(msg); ok {
			out = append(out, turn)
		}
	}
	return out
}

// promptHistory is the history sent to the model for the current session.
func promptHistory(messages []store.Message) []prompt.Turn {
	return HistoryTurns(visibleForPrompt(messages))
}

// liveMessages drops deleted and already compacted messages.
func liveMessages(messages []store.Message) []store.Message {
	out := make([]store.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.DeletedAtMs == 0 && !msg.Compacted {
			out = append(out, msg)
		}
	}
	return out
}

// maxInt returns the larger of two ints.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// turnFromMessage decodes the stored provider turn, falling back to visible plain-text rows.
func turnFromMessage(msg store.Message) (prompt.Turn, bool) {
	if turn, ok := prompt.DecodeTurn(msg.APIContent); ok {
		if turn.Role == "" {
			turn.Role = msg.Role
		}
		return turn, true
	}
	if !msg.Visible {
		return prompt.Turn{}, false
	}
	switch msg.Role {
	case store.RoleUser, store.RoleAssistant, store.RoleTool:
		return prompt.Turn{Role: msg.Role, Content: msg.Content}, true
	default:
		return prompt.Turn{}, false
	}
}
