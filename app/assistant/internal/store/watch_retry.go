package store

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	watchDeliveryMaxAttempts = 8
	watchRetryBaseDelay      = time.Minute
	watchRetryMaxDelay       = 30 * time.Minute
	watchDeliveryRetention   = 90 * 24 * time.Hour
)

func watchTerminalBucketState(runStatus string, bucket DeliveryBucket, failedAttempts int, nowMs int64) (string, int64, error) {
	switch runStatus {
	case StatusCancelled:
		return "pending", 0, nil
	case StatusError:
		status, notBeforeMs := failedWatchBucketState(bucket, failedAttempts, nowMs)
		return status, notBeforeMs, nil
	default:
		return "", 0, fmt.Errorf("invalid terminal watch run status %q", runStatus)
	}
}

func failedWatchBucketState(bucket DeliveryBucket, failedAttempts int, nowMs int64) (string, int64) {
	if failedAttempts < 1 {
		failedAttempts = 1
	}
	if failedAttempts >= watchDeliveryMaxAttempts {
		return "discarded", 0
	}

	delay := watchRetryBaseDelay * time.Duration(1<<(failedAttempts-1))
	if delay > watchRetryMaxDelay {
		delay = watchRetryMaxDelay
	}
	notBeforeMs := nowMs + delay.Milliseconds()
	if bucket.CreatedAtMs > 0 {
		expiresAtMs := bucket.CreatedAtMs + watchDeliveryRetention.Milliseconds()
		if nowMs >= expiresAtMs || notBeforeMs >= expiresAtMs {
			return "discarded", 0
		}
	}
	return "deferred", notBeforeMs
}

func watchBucketIDFromPayload(payload []byte) int64 {
	var parsed struct {
		BucketID int64 `json:"bucket_id"`
	}
	if json.Unmarshal(payload, &parsed) != nil {
		return 0
	}
	return parsed.BucketID
}
