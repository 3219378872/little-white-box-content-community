package event

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const BehaviorSchemaVersion int32 = 2

const (
	BehaviorActionExposure   = "exposure"
	BehaviorActionClick      = "click"
	BehaviorActionDwell      = "dwell"
	BehaviorActionPlay       = "play"
	BehaviorActionView       = "view"
	BehaviorActionLike       = "like"
	BehaviorActionUnlike     = "unlike"
	BehaviorActionFavorite   = "favorite"
	BehaviorActionUnfavorite = "unfavorite"
	BehaviorActionComment    = "comment"
	BehaviorActionFollow     = "follow"
	BehaviorActionUnfollow   = "unfollow"
	BehaviorActionShare      = "share"
	BehaviorActionHide       = "hide"
	BehaviorActionDislike    = "dislike"
)

var supportedBehaviorActions = map[string]struct{}{
	BehaviorActionExposure: {}, BehaviorActionClick: {}, BehaviorActionDwell: {},
	BehaviorActionPlay: {}, BehaviorActionView: {}, BehaviorActionLike: {},
	BehaviorActionUnlike: {}, BehaviorActionFavorite: {}, BehaviorActionUnfavorite: {},
	BehaviorActionComment: {}, BehaviorActionFollow: {}, BehaviorActionUnfollow: {},
	BehaviorActionShare: {}, BehaviorActionHide: {}, BehaviorActionDislike: {},
}

// clientAllowedBehaviorActions 是客户端可提交的动作白名单（REL-001）。
// like/unlike/favorite/unfavorite/comment/follow/unfollow 只允许由权威业务事务
// 经 outbox 生成，客户端直接提交时必须拒绝。
var clientAllowedBehaviorActions = map[string]struct{}{
	BehaviorActionExposure: {},
	BehaviorActionClick:    {},
	BehaviorActionDwell:    {},
	BehaviorActionPlay:     {},
	BehaviorActionView:     {},
	BehaviorActionShare:    {},
	BehaviorActionHide:     {},
	BehaviorActionDislike:  {},
}

var durationBehaviorActions = map[string]struct{}{
	BehaviorActionDwell: {},
	BehaviorActionPlay:  {},
	BehaviorActionView:  {},
}

// BehaviorEvent is the canonical v2 payload stored and consumed unchanged.
type BehaviorEvent struct {
	EventID       int64  `json:"event_id"`
	ClientEventID string `json:"client_event_id"`
	SchemaVersion int32  `json:"schema_version"`
	EventTime     int64  `json:"event_time"`
	ReceivedAt    int64  `json:"received_at"`
	UserID        int64  `json:"user_id"`
	AnonymousID   string `json:"anonymous_id"`
	SessionID     string `json:"session_id"`
	RequestID     string `json:"request_id"`
	Action        string `json:"action"`
	TargetID      int64  `json:"target_id"`
	TargetType    string `json:"target_type"`
	Scene         string `json:"scene"`
	Position      *int32 `json:"position,omitempty"`
	DurationMs    *int64 `json:"duration_ms,omitempty"`
	RecallSource  string `json:"recall_source"`
	ModelVersion  string `json:"model_version"`
	ExperimentID  string `json:"experiment_id"`
	Producer      string `json:"producer"`
	ClientIP      string `json:"client_ip"`
	ClientVersion string `json:"client_version"`
}

func (e BehaviorEvent) Validate() error {
	if e.EventID <= 0 {
		return fmt.Errorf("event_id is required")
	}
	if strings.TrimSpace(e.ClientEventID) == "" {
		return fmt.Errorf("client_event_id is required")
	}
	if len(e.ClientEventID) > 128 {
		return fmt.Errorf("client_event_id is too long")
	}
	if e.SchemaVersion != BehaviorSchemaVersion {
		return fmt.Errorf("schema_version must be %d", BehaviorSchemaVersion)
	}
	if e.EventTime <= 0 {
		return fmt.Errorf("event_time is required")
	}
	if e.ReceivedAt <= 0 {
		return fmt.Errorf("received_at is required")
	}
	if e.UserID <= 0 && strings.TrimSpace(e.AnonymousID) == "" {
		return fmt.Errorf("user_id or anonymous_id is required")
	}
	if _, ok := supportedBehaviorActions[e.Action]; !ok {
		return fmt.Errorf("action is unsupported")
	}
	if e.TargetID <= 0 {
		return fmt.Errorf("target_id is required")
	}
	if strings.TrimSpace(e.TargetType) == "" {
		return fmt.Errorf("target_type is required")
	}
	if strings.TrimSpace(e.Producer) == "" {
		return fmt.Errorf("producer is required")
	}
	if e.Position != nil && *e.Position < 0 {
		return fmt.Errorf("position must not be negative")
	}
	if e.Action == BehaviorActionExposure {
		if strings.TrimSpace(e.RequestID) == "" {
			return fmt.Errorf("request_id is required for exposure")
		}
		if strings.TrimSpace(e.Scene) == "" {
			return fmt.Errorf("scene is required for exposure")
		}
		if e.Position == nil {
			return fmt.Errorf("position is required for exposure")
		}
		if *e.Position < 1 {
			return fmt.Errorf("position must start from 1 for exposure")
		}
	}
	if e.DurationMs != nil {
		if *e.DurationMs < 0 {
			return fmt.Errorf("duration_ms must not be negative")
		}
		if _, ok := durationBehaviorActions[e.Action]; !ok {
			return fmt.Errorf("duration_ms is not allowed for action %s", e.Action)
		}
	}
	if _, ok := durationBehaviorActions[e.Action]; ok && e.DurationMs == nil {
		return fmt.Errorf("duration_ms is required for action %s", e.Action)
	}
	return nil
}

// ValidateClientSubmitted 校验客户端提交的事件（REL-001）：先执行通用校验，
// 再强制动作白名单。权威业务动作只能由服务端 outbox 生成。
func (e BehaviorEvent) ValidateClientSubmitted() error {
	if err := e.Validate(); err != nil {
		return err
	}
	if _, ok := clientAllowedBehaviorActions[e.Action]; !ok {
		return fmt.Errorf("action %s is not allowed from clients", e.Action)
	}
	return nil
}

// DeterministicBehaviorEventID keeps redelivery idempotent when the broker ACK
// succeeds but the RPC response is lost.
func DeterministicBehaviorEventID(clientEventID string) int64 {
	digest := sha256.Sum256([]byte(clientEventID))
	id := int64(binary.BigEndian.Uint64(digest[:8]) & math.MaxInt64)
	if id == 0 {
		return 1
	}
	return id
}

func (e BehaviorEvent) EventIDString() string {
	return strconv.FormatInt(e.EventID, 10)
}
