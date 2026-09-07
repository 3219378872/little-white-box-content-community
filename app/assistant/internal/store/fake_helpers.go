package store

import (
	"encoding/json"
)

func sortMessages(msgs []Message) {
	for i := 0; i < len(msgs); i++ {
		for j := i + 1; j < len(msgs); j++ {
			if msgs[j].ID < msgs[i].ID {
				msgs[i], msgs[j] = msgs[j], msgs[i]
			}
		}
	}
}

func toolKey(runID int64, callID string) string {
	return itoa(runID) + ":" + callID
}

func journalKey(userID int64, requestID, tool, digest string) string {
	return itoa(userID) + ":" + requestID + ":" + tool + ":" + digest
}

func sourceKey(runID int64, handle string) string { return itoa(runID) + ":" + handle }

func confirmKey(runID int64, callID string) string {
	return itoa(runID) + ":" + callID
}

func inputCommandKey(userID int64, requestID string) string {
	return itoa(userID) + ":" + requestID
}

func alertKey(runID int64, level, dim string) string {
	return itoa(runID) + ":" + level + ":" + dim
}

func bucketKey(userID, window int64) string { return itoa(userID) + ":" + itoa(window) }

func sentKey(userID, taskID int64, kind string, start int64) string {
	return itoa(userID) + ":" + itoa(taskID) + ":" + kind + ":" + itoa(start)
}

func itoa(v int64) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
