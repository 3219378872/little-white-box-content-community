package store

import (
	"context"
	"strconv"
)

func (s *SQLStore) ListHistoryAround(ctx context.Context, userID, messageID int64, before, after int, cutoffMs int64, excludeIDs []int64) ([]Message, error) {
	before = boundedHistoryEdge(before)
	after = boundedHistoryEdge(after)
	where, args := historyWhere(userID, cutoffMs, excludeIDs)
	anchorArgs := append(append([]any(nil), args...), messageID)
	anchor, err := s.scanMessages(ctx, `SELECT id, user_id, session_id, run_id, role, kind, content, api_content, visible, unread, compacted, change_id, deleted_at_ms, created_at_ms
		FROM assistant_message WHERE `+where+` AND id=?`, anchorArgs...)
	if err != nil || len(anchor) == 0 {
		return nil, err
	}
	where += ` AND session_id=?`
	args = append(args, anchor[0].SessionID)
	beforeArgs := append(append([]any(nil), args...), messageID)
	preceding, err := s.scanMessages(ctx, `SELECT id, user_id, session_id, run_id, role, kind, content, api_content, visible, unread, compacted, change_id, deleted_at_ms, created_at_ms
		FROM assistant_message WHERE `+where+` AND id<? ORDER BY id DESC LIMIT `+strconv.Itoa(before), beforeArgs...)
	if err != nil {
		return nil, err
	}
	reverseMessages(preceding)
	afterArgs := append(append([]any(nil), args...), messageID)
	following, err := s.scanMessages(ctx, `SELECT id, user_id, session_id, run_id, role, kind, content, api_content, visible, unread, compacted, change_id, deleted_at_ms, created_at_ms
		FROM assistant_message WHERE `+where+` AND id>? ORDER BY id ASC LIMIT `+strconv.Itoa(after), afterArgs...)
	if err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(preceding)+1+len(following))
	out = append(out, preceding...)
	out = append(out, anchor[0])
	out = append(out, following...)
	return out, nil
}

func (s *SQLStore) ListHistorySessionSummaries(ctx context.Context, userID, sessionID int64, limit int, cutoffMs int64, excludeIDs []int64) ([]HistorySessionSummary, error) {
	if limit <= 0 {
		limit = 3
	}
	if limit > 10 {
		limit = 10
	}
	where, args := historyWhere(userID, cutoffMs, excludeIDs)
	if sessionID > 0 {
		where += ` AND session_id=?`
		args = append(args, sessionID)
	}
	var groups []struct {
		SessionID int64 `db:"session_id"`
		FirstID   int64 `db:"first_id"`
		LastID    int64 `db:"last_id"`
		LastAtMs  int64 `db:"last_at_ms"`
	}
	if err := s.exec.QueryRowsCtx(ctx, &groups, `SELECT session_id, MIN(id) AS first_id, MAX(id) AS last_id,
		MAX(created_at_ms) AS last_at_ms FROM assistant_message WHERE `+where+`
		GROUP BY session_id ORDER BY last_at_ms DESC, session_id DESC LIMIT `+strconv.Itoa(limit), args...); err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(groups)*2)
	for _, group := range groups {
		ids = append(ids, group.FirstID)
		if group.LastID != group.FirstID {
			ids = append(ids, group.LastID)
		}
	}
	messages, err := s.GetMessagesByIDs(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	excluded := int64Set(excludeIDs)
	byID := make(map[int64]Message, len(messages))
	for _, message := range messages {
		if !historyMessageEligible(message, userID, cutoffMs, excluded) {
			continue
		}
		byID[message.ID] = message
	}
	out := make([]HistorySessionSummary, 0, len(groups))
	for _, group := range groups {
		first, firstOK := byID[group.FirstID]
		last, lastOK := byID[group.LastID]
		if !firstOK || !lastOK {
			continue
		}
		out = append(out, HistorySessionSummary{SessionID: group.SessionID, First: first, Last: last, LastAtMs: group.LastAtMs})
	}
	return out, nil
}

func historyWhere(userID, cutoffMs int64, excludeIDs []int64) (string, []any) {
	where := `user_id=? AND deleted_at_ms IS NULL AND visible=1
		AND role IN ('user','assistant') AND kind IN ('message','watch') AND created_at_ms>=?`
	args := []any{userID, cutoffMs}
	if len(excludeIDs) > 0 {
		where += ` AND id NOT IN (` + placeholders(len(excludeIDs)) + `)`
		args = append(args, intsToAny(excludeIDs)...)
	}
	return where, args
}

func boundedHistoryEdge(value int) int {
	if value <= 0 {
		return 5
	}
	if value > 20 {
		return 20
	}
	return value
}

func historyMessageEligible(message Message, userID, cutoffMs int64, excluded map[int64]struct{}) bool {
	if message.UserID != userID || message.DeletedAtMs != 0 || !message.Visible || message.CreatedAtMs < cutoffMs {
		return false
	}
	if message.Role != RoleUser && message.Role != RoleAssistant {
		return false
	}
	if message.Kind != KindMessage && message.Kind != KindWatch {
		return false
	}
	_, blocked := excluded[message.ID]
	return !blocked
}

func int64Set(values []int64) map[int64]struct{} {
	out := make(map[int64]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}
