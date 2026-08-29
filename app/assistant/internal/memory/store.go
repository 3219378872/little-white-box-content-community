package memory

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"esx/app/assistant/internal/safety"
	"esx/pkg/errx"
)

const (
	TargetMemory = "memory"
	TargetUser   = "user"

	CapacityMemory = 2200
	CapacityUser   = 1375

	OpAdd     = "add"
	OpReplace = "replace"
	OpRemove  = "remove"
)

type Entry struct {
	ID          int64
	UserID      int64
	Target      string
	Content     string
	Version     int32
	CreatedAtMs int64
	UpdatedAtMs int64
	Deleted     bool
}

type Change struct {
	ID            int64
	UserID        int64
	EntryID       int64
	Op            string
	Before        *Entry
	After         *Entry
	ResultVersion int32
	RequestID     string
	Undone        bool
	CreatedAtMs   int64
}

type Capacity struct {
	Target string
	Used   int
	Limit  int
}

type Op struct {
	Op      string
	ID      int64
	Target  string
	Content string
	Version int32
}

type Store interface {
	List(ctx context.Context, userID int64, target string) ([]Entry, []Capacity, error)
	Add(ctx context.Context, userID int64, target, content, requestID string, nowMs int64) (Entry, int64, error)
	Replace(ctx context.Context, userID, id int64, content string, version int32, requestID string, nowMs int64) (Entry, int64, error)
	Remove(ctx context.Context, userID, id int64, version int32, requestID string, nowMs int64) (int64, error)
	Batch(ctx context.Context, userID int64, requestID string, ops []Op, nowMs int64) ([]Entry, []int64, error)
	Undo(ctx context.Context, userID, changeID int64, nowMs int64) (*Entry, error)
	Active(ctx context.Context, userID int64) ([]Entry, error)
	RecordFeedback(ctx context.Context, userID int64, requestID string, postID int64, reason string) error
}

type Scanner interface {
	Check(ctx context.Context, text string) error
}

func Normalize(content string) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(strings.TrimSpace(content)) {
		if unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	return strings.TrimSpace(b.String())
}

func LimitFor(target string) int {
	if target == TargetUser {
		return CapacityUser
	}
	return CapacityMemory
}

func ValidTarget(target string) bool {
	return target == TargetMemory || target == TargetUser
}

func ScanContent(ctx context.Context, scanner Scanner, content string) error {
	if strings.TrimSpace(content) == "" {
		return errx.New(errx.ParamError, "memory content is required")
	}
	lower := strings.ToLower(content)
	for _, needle := range []string{
		"ignore previous instructions", "ignore all previous", "system prompt",
		"you are now", "disregard the above", "忽略以上", "忽略之前",
	} {
		if strings.Contains(lower, needle) {
			return errx.New(errx.ParamError, "memory content failed threat scan")
		}
	}
	if scanner != nil {
		if err := scanner.Check(ctx, content); err != nil {
			if err == safety.ErrBlocked {
				return errx.New(errx.ParamError, "memory content failed threat scan")
			}
			return err
		}
	}
	return nil
}

func UsedRunes(entries []Entry, target string) int {
	n := 0
	for _, item := range entries {
		if item.Deleted || item.Target != target {
			continue
		}
		n += utf8.RuneCountInString(item.Content)
	}
	return n
}
