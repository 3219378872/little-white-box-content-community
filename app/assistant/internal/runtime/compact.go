package runtime

import (
	"strings"

	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
)

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

func ShouldCompact(messages []store.Message, windowTokens int, runID int64) bool {
	return ShouldCompactWithAnchor(messages, windowTokens, runID, 0)
}

func ShouldCompactWithAnchor(messages []store.Message, windowTokens int, runID int64, lastPromptTokens int64) bool {
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
	selected := SelectKeep(live, maxInt(total/5, 1), unfinishedCallIDs(live), runID)
	return len(selected) < len(live)
}

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

func EstimatePromptTokens(turns []prompt.Turn) int {
	total := 0
	for _, turn := range turns {
		total += EstimateTokens(string(prompt.EncodeTurn(turn)))
	}
	return total
}

func estimateStoredTokens(msg store.Message) int {
	if len(msg.APIContent) > 0 {
		return EstimateTokens(string(msg.APIContent))
	}
	return EstimateTokens(msg.Content)
}

func SelectKeep(messages []store.Message, keepTokens int, unfinished map[string]struct{}, runID int64) []store.Message {
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
		force := messageHasUnfinishedCall(msg, unfinished) || messageIsLiveWatchInput(msg, runID)
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
		if msg.DeletedAtMs != 0 || msg.Compacted || msg.Kind == store.KindQuestion {
			continue
		}
		if turn, ok := turnFromMessage(msg); ok {
			out = append(out, turn)
		}
	}
	return out
}

func promptHistory(messages []store.Message, run store.Run) []prompt.Turn {
	live := visibleForPrompt(messages)
	if run.Source == store.SourceWatch {
		live = placeWatchInput(live, run.ID)
	}
	return HistoryTurns(live)
}

func placeWatchInput(messages []store.Message, runID int64) []store.Message {
	if runID <= 0 {
		return messages
	}
	input := make([]store.Message, 0, 1)
	rest := make([]store.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.RunID == runID && msg.Kind == store.KindWatchInput {
			input = append(input, msg)
			continue
		}
		rest = append(rest, msg)
	}
	if len(input) == 0 {
		return messages
	}
	out := make([]store.Message, 0, len(messages))
	placed := false
	for _, msg := range rest {
		if !placed && msg.RunID == runID {
			out = append(out, input...)
			placed = true
		}
		out = append(out, msg)
	}
	if !placed {
		out = append(out, input...)
	}
	return out
}

func messageIsLiveWatchInput(msg store.Message, runID int64) bool {
	return runID > 0 && msg.RunID == runID && msg.Kind == store.KindWatchInput
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
