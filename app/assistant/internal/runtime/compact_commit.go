package runtime

import (
	"context"
	"errors"
	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"strings"
)

func (e *Engine) compact(workCtx, persistCtx context.Context, run *store.Run, session *store.Session, msgs []store.Message, mainClient llm.Client) error {
	run.Phase = store.PhaseCompact
	if err := e.updateRun(persistCtx, *run); err != nil {
		return err
	}
	msgs = liveMessages(msgs)
	keep := EstimateMessageTokens(msgs) / 5
	if keep < 1 {
		keep = 1
	}
	selected := SelectKeep(msgs, keep, unfinishedCallIDs(msgs), run.ID)
	keepIDs := make(map[int64]struct{}, len(selected))
	for _, msg := range selected {
		keepIDs[msg.ID] = struct{}{}
	}
	dropped := make([]store.Message, 0, len(msgs)-len(selected))
	for _, msg := range msgs {
		if _, keep := keepIDs[msg.ID]; !keep {
			dropped = append(dropped, msg)
		}
	}
	summary, err := e.summarizeCompaction(workCtx, persistCtx, run, dropped, mainClient)
	if err != nil {
		return err
	}

	if e.cancelled(persistCtx, run) {
		return errRunCancelled
	}
	capability, err := e.buildCapabilitySnapshot(*run)
	if err != nil {
		return err
	}
	toolSnapshot := prompt.EncodeCapabilities(capability)
	ids := make([]int64, 0, len(msgs))
	for _, msg := range msgs {
		if msg.Compacted {
			continue
		}
		if _, keep := keepIDs[msg.ID]; keep {
			continue
		}
		ids = append(ids, msg.ID)
	}
	var entries []memory.Entry
	if e.Memory != nil {
		var err error
		entries, err = e.Memory.Active(persistCtx, run.UserID)
		if err != nil {
			return err
		}
	}
	snap := prompt.BuildSnapshot(entries, HistoryTurns(selected), summary)
	beforeSnapshot, ok := prompt.DecodeSnapshot(session.PromptSnapshot)
	if !ok {
		return errors.New("assistant prompt snapshot is invalid before compact")
	}
	beforeSnapshot.History = HistoryTurns(msgs)
	beforeTokens := EstimatePromptTokens(prompt.Messages(beforeSnapshot)) + EstimateTokens(string(session.ToolSnapshot))
	if run.LastPromptTokens > int64(beforeTokens) {
		beforeTokens = int(run.LastPromptTokens)
	}
	afterTokens := EstimatePromptTokens(prompt.Messages(snap)) + EstimateTokens(string(toolSnapshot))
	window := e.Window
	if mainClient != nil && mainClient.ContextWindowTokens() > 0 {
		window = mainClient.ContextWindowTokens()
	}
	target := window / 2
	if target <= 0 {
		target = 64_000
	}
	if afterTokens >= beforeTokens || afterTokens >= target {
		return errCompactNoGain
	}
	session.PromptEpoch++
	session.PromptSnapshot = prompt.EncodeSnapshot(snap)
	session.ToolSnapshot = toolSnapshot
	session.CompactSummary = summary
	run.PromptEpoch = session.PromptEpoch
	run.LastPromptTokens = int64(afterTokens)
	run.Phase = store.PhaseModelRequest
	return e.step(persistCtx, *run, func(ctx context.Context, tx store.Store) error {
		if len(ids) > 0 {
			if err := tx.MarkMessagesCompacted(ctx, ids); err != nil {
				return err
			}
		}
		if err := tx.UpdateSession(ctx, *session); err != nil {
			return err
		}
		return tx.UpdateRun(ctx, *run)
	})
}

func (e *Engine) summarizeCompaction(workCtx, persistCtx context.Context, run *store.Run, dropped []store.Message, mainClient llm.Client) (string, error) {
	summary := "压缩摘要：较早对话未包含可保留的用户可见内容。"
	summaryClient := e.AuxLLM
	if summaryClient == nil {
		summaryClient = mainClient
	}
	if summaryClient != nil {
		budget := summaryClient.ContextWindowTokens() / 4
		if budget <= 0 || budget > 32_000 {
			budget = 32_000
		}
		if budget < 2_000 {
			budget = 2_000
		}
		input := SummaryInput(dropped, budget)
		if input != "" {
			result, err := summaryClient.Complete(workCtx, llm.Request{
				Messages:     []prompt.Turn{{Role: store.RoleSystem, Content: "用中文压缩以下会话，不要引入新事实。"}, {Role: store.RoleUser, Content: input}},
				DisableTools: true,
				MaxTokens:    512,
			})
			if err != nil {
				if errors.Is(err, context.Canceled) && e.cancelled(persistCtx, run) {
					return "", errRunCancelled
				}
				return "", err
			}
			if strings.TrimSpace(result.Text) == "" {
				return "", errCompactNoGain
			}
			summary = prompt.SanitizeOutput(result.Text)
			run.InputTokens += result.Usage.PromptTokens
			run.OutputTokens += result.Usage.CompletionTokens
			run.CacheTokens += result.Usage.CacheTokens
			run.CacheWriteTokens += result.Usage.CacheWriteTokens
			run.ReasoningTokens += result.Usage.ReasoningTokens
			run.UsageEstimated = run.UsageEstimated || result.Usage.Estimated
			run.CostUSD += result.Usage.CostUSD
		}
	}
	return summary, nil
}
