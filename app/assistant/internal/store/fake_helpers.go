package store

import (
	"strconv"
)

// sortMessages orders messages by ID, like the SQL ORDER BY id.
func sortMessages(msgs []Message) {
	for i := 0; i < len(msgs); i++ {
		for j := i + 1; j < len(msgs); j++ {
			if msgs[j].ID < msgs[i].ID {
				msgs[i], msgs[j] = msgs[j], msgs[i]
			}
		}
	}
}

// toolKey mirrors the (run_id, call_id) key of agent_tool_call.
func toolKey(runID int64, callID string) string {
	return strconv.FormatInt(runID, 10) + ":" + callID
}

// journalKey mirrors uk_journal of agent_command_journal.
func journalKey(userID int64, requestID, tool, digest string) string {
	return strconv.FormatInt(userID, 10) + ":" + requestID + ":" + tool + ":" + digest
}

// sourceKey mirrors uk_source_handle of agent_source_ledger.
func sourceKey(runID int64, handle string) string { return strconv.FormatInt(runID, 10) + ":" + handle }

// confirmKey mirrors the (run_id, call_id) lookup of agent_confirmation.
func confirmKey(runID int64, callID string) string {
	return strconv.FormatInt(runID, 10) + ":" + callID
}

// inputCommandKey mirrors uk_input_command_user_req of assistant_input_command.
func inputCommandKey(userID int64, requestID string) string {
	return strconv.FormatInt(userID, 10) + ":" + requestID
}

// alertKey mirrors the primary key of agent_run_alert.
func alertKey(runID int64, level, dim string) string {
	return strconv.FormatInt(runID, 10) + ":" + level + ":" + dim
}
