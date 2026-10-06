package store

import (
	"strconv"
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
	return strconv.FormatInt(runID, 10) + ":" + callID
}

func journalKey(userID int64, requestID, tool, digest string) string {
	return strconv.FormatInt(userID, 10) + ":" + requestID + ":" + tool + ":" + digest
}

func sourceKey(runID int64, handle string) string { return strconv.FormatInt(runID, 10) + ":" + handle }

func confirmKey(runID int64, callID string) string {
	return strconv.FormatInt(runID, 10) + ":" + callID
}

func inputCommandKey(userID int64, requestID string) string {
	return strconv.FormatInt(userID, 10) + ":" + requestID
}

func alertKey(runID int64, level, dim string) string {
	return strconv.FormatInt(runID, 10) + ":" + level + ":" + dim
}
