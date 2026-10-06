package memory

import (
	"context"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"esx/app/assistant/internal/safety"
	"esx/pkg/errx"
)

const (
	// 记忆分为 memory（助手笔记）与 user（用户画像）两个目标，各有字符容量上限。
	TargetMemory = "memory"
	TargetUser   = "user"

	CapacityMemory = 2200
	CapacityUser   = 1375

	OpAdd     = "add"
	OpReplace = "replace"
	OpRemove  = "remove"
)

// 记忆错误的用户文案：经 REST 直接展示给用户，同样的错误也作为工具结果回给模型；
// 两个实现共用同一组文案，错误码不变。
const (
	msgInvalidTarget    = "记忆类型只能是 memory 或 user"
	msgUnknownOp        = "不支持的记忆操作"
	msgCapacityExceeded = "记忆容量已满，请先删除或精简部分记忆"
	msgVersionConflict  = "记忆已被修改，请刷新后重试"
	msgDuplicateContent = "已有相同内容的记忆"
	msgAlreadyUndone    = "这次修改已经撤销"
	msgRestoreConflict  = "相同内容的记忆已存在，无法撤销"
	msgContentRequired  = "记忆内容不能为空"
	msgThreatScan       = "记忆内容未通过安全检查"
)

// Entry 是一条记忆；Version 用于乐观并发，删除为软删除。
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

// Change 记录一次变更的前后状态，供撤销与按 requestID 幂等重放。
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

// Capacity 是某个目标已用与上限字符数。
type Capacity struct {
	Target string
	Used   int
	Limit  int
}

// Op 是批量操作中的一条增删改。
type Op struct {
	Op      string
	ID      int64
	Target  string
	Content string
	Version int32
}

// Store 是记忆的读写接口；增删改操作按 requestID 幂等。
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

// Scanner 是写入前的内容安全检查。
type Scanner interface {
	Check(ctx context.Context, text string) error
}

// Normalize 统一大小写与空白，用于判断两条记忆是否重复。
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

// LimitFor 返回目标的字符容量上限。
func LimitFor(target string) int {
	if target == TargetUser {
		return CapacityUser
	}
	return CapacityMemory
}

// ValidTarget 判断目标是否合法。
func ValidTarget(target string) bool {
	return target == TargetMemory || target == TargetUser
}

// ScanContent 拒绝空内容与常见提示词注入短语，再交给可选的安全过滤器，防止记忆成为注入通道。
func ScanContent(ctx context.Context, scanner Scanner, content string) error {
	if strings.TrimSpace(content) == "" {
		return errx.New(errx.ParamError, msgContentRequired)
	}
	lower := strings.ToLower(content)
	for _, needle := range []string{
		"ignore previous instructions", "ignore all previous", "system prompt",
		"you are now", "disregard the above", "忽略以上", "忽略之前",
	} {
		if strings.Contains(lower, needle) {
			return errx.New(errx.ParamError, msgThreatScan)
		}
	}
	if scanner != nil {
		if err := scanner.Check(ctx, content); err != nil {
			if err == safety.ErrBlocked {
				return errx.New(errx.ParamError, msgThreatScan)
			}
			return err
		}
	}
	return nil
}

// UsedRunes 统计目标下有效条目的字符数。
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

// opRequestID 给批量操作的每一条派生独立的幂等键（`<requestID>#<序号>`）；单条操作直接使用原键。
func opRequestID(requestID string, index, total int) string {
	if total > 1 {
		return requestID + "#" + strconv.FormatInt(int64(index), 10)
	}
	return requestID
}

// replayResult 把同一 requestID 已提交的变更还原为本次调用的返回值（两种存储共用的幂等重放）。
// 参数与原变更不一致时报幂等冲突，而不是悄悄返回旧结果。
func replayResult(op Op, change Change) (*Entry, int64, error) {
	if !memoryReplayMatches(op, change) {
		return nil, 0, errx.NewWithCode(errx.IdempotencyConflict)
	}
	// 删除操作不返回条目，只返回变更 ID。
	if strings.EqualFold(strings.TrimSpace(op.Op), OpRemove) {
		return nil, change.ID, nil
	}
	if change.After == nil {
		return nil, 0, errx.NewWithCode(errx.IdempotencyConflict)
	}
	entry := *change.After
	return &entry, change.ID, nil
}

// memoryReplayMatches 判断本次操作与已记录变更是否是同一请求：操作类型、目标条目、
// 期望版本与规范化内容都必须一致；已撤销的变更不能被重放。
func memoryReplayMatches(op Op, change Change) bool {
	wantOp := strings.ToLower(strings.TrimSpace(op.Op))
	if wantOp == "" {
		wantOp = OpAdd
	}
	if change.Op != wantOp || change.Undone {
		return false
	}
	switch wantOp {
	case OpAdd:
		return change.After != nil && change.After.Target == op.Target && Normalize(change.After.Content) == Normalize(op.Content)
	case OpReplace:
		return change.Before != nil && change.After != nil && change.EntryID == op.ID &&
			change.Before.Version == op.Version && Normalize(change.After.Content) == Normalize(op.Content)
	case OpRemove:
		return change.Before != nil && change.After != nil && change.EntryID == op.ID &&
			change.Before.Version == op.Version && change.After.Deleted
	default:
		return false
	}
}
