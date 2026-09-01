package store

import (
	"errors"
	"time"
)

const (
	SourceUser         = "user"
	SourceWatch        = "watch"
	SourceMemoryReview = "memory-review"

	PriorityUser         = 0
	PriorityWatch        = 10
	PriorityMemoryReview = 20

	PhaseQueued        = "queued"
	PhaseModelRequest  = "model_request"
	PhaseToolExecuting = "tool_executing"
	PhaseCompact       = "compact"
	PhaseAttachment    = "attachment"
	PhaseDone          = "done"

	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusDone      = "done"
	StatusError     = "error"
	StatusCancelled = "cancelled"

	DispositionStarted    = "started"
	DispositionRedirected = "redirected"
	DispositionSteered    = "steered"
	DispositionQueued     = "queued"

	EventRunStarted      = "run_started"
	EventToken           = "token"
	EventResponseReset   = "response_reset"
	EventProviderAttempt = "provider_attempt"
	EventToolCall        = "tool_call"
	EventToolResult      = "tool_result"
	EventConfirmRequired = "confirm_required"
	EventSourceCard      = "source_card"
	EventMemoryChanged   = "memory_changed"
	EventDone            = "done"
	EventError           = "error"

	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"

	KindMessage       = "message"
	KindMemoryChanged = "memory_changed"
	KindWatch         = "watch"
	KindWatchInput    = "watch_input"
	KindTool          = "tool"

	SessionOpen   = "open"
	SessionClosed = "closed"

	ConfirmPending  = "pending"
	ConfirmApproved = "approved"
	ConfirmRejected = "rejected"

	JournalPending = "pending"
	JournalSuccess = "success"
	JournalError   = "error"

	MaxInputQueue = 32

	IndexOpUpsert = "upsert"
	IndexOpDelete = "delete"
)

var ErrLeaseLost = errors.New("assistant run lease lost")

type LeaseFence struct {
	RunID      int64
	Owner      string
	Generation int64
}

type Thread struct {
	UserID             int64
	SessionID          int64
	UnreadCount        int32
	LastMessageID      int64
	LastMessagePreview string
	LastMessageAtMs    int64
	ActiveRunID        int64
	UpdatedAtMs        int64
}

type Session struct {
	ID                  int64
	UserID              int64
	PromptEpoch         int
	PromptSnapshot      []byte
	ToolSnapshot        []byte
	CompactSummary      string
	Status              string
	SuccessfulUserTurns int
	CreatedAtMs         int64
	ClosedAtMs          int64
}

type Message struct {
	ID          int64
	UserID      int64
	SessionID   int64
	RunID       int64
	Role        string
	Kind        string
	Content     string
	APIContent  []byte
	Visible     bool
	Unread      bool
	Compacted   bool
	DeletedAtMs int64
	CreatedAtMs int64
	ChangeID    int64
}

type Run struct {
	ID               int64
	UserID           int64
	SessionID        int64
	RequestID        string
	Source           string
	Status           string
	Phase            string
	Priority         int
	QueuedPayload    []byte
	LeaseOwner       string
	LeaseGeneration  int64
	LeaseUntilMs     int64
	HeartbeatAtMs    int64
	CancelRequested  bool
	ConsentVersion   int32
	InputVersion     int64
	PromptEpoch      int
	Model            string
	Rounds           int
	ToolCalls        int
	InputTokens      int64
	OutputTokens     int64
	CacheTokens      int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	LastPromptTokens int64
	UsageEstimated   bool
	CostUSD          float64
	StartedAtMs      int64
	EndedAtMs        int64
	LastActivityAtMs int64
	ErrorCode        string
	CreatedAtMs      int64
}

type Event struct {
	ID          int64
	RunID       int64
	Seq         int64
	Type        string
	PayloadJSON []byte
	CreatedAtMs int64
}

type EventPayload struct {
	Text       string     `json:"text,omitempty"`
	Degraded   bool       `json:"degraded,omitempty"`
	ErrorCode  string     `json:"error_code,omitempty"`
	SessionID  int64      `json:"session_id,omitempty"`
	ToolCall   *ToolInfo  `json:"tool_call,omitempty"`
	SourceCard *SourceRef `json:"source_card,omitempty"`
	ChangeID   int64      `json:"change_id,omitempty"`
	Partial    string     `json:"partial,omitempty"`
	Journal    string     `json:"journal,omitempty"`
	StreamID   string     `json:"stream_id,omitempty"`
	RouteID    string     `json:"route_id,omitempty"`
	Attempt    int        `json:"attempt,omitempty"`
	ErrorClass string     `json:"error_class,omitempty"`
	StatusCode int        `json:"status_code,omitempty"`
	Retryable  bool       `json:"retryable,omitempty"`
}

type ToolInfo struct {
	CallID      string `json:"call_id,omitempty"`
	Tool        string `json:"tool,omitempty"`
	Summary     string `json:"summary,omitempty"`
	PayloadJSON string `json:"payload_json,omitempty"`
}

type SourceRef struct {
	Handle      string `json:"handle"`
	Kind        string `json:"kind"`
	AuthorityID string `json:"authority_id"`
	Title       string `json:"title"`
	Revision    int64  `json:"revision"`
	PayloadJSON string `json:"payload_json,omitempty"`
	Available   bool   `json:"available,omitempty"`
}

type ToolCall struct {
	ID                  int64
	RunID               int64
	CallID              string
	Tool                string
	ArgsJSON            string
	CanonicalArgsDigest string
	Status              string
	ResultJSON          string
	CreatedAtMs         int64
}

type Journal struct {
	ID                  int64
	UserID              int64
	RequestID           string
	Tool                string
	CanonicalArgsDigest string
	RunID               int64
	LeaseGeneration     int64
	ResultJSON          string
	Status              string
	CreatedAtMs         int64
	UpdatedAtMs         int64
	Takeover            bool
}

func (r Run) Fence() LeaseFence {
	return LeaseFence{RunID: r.ID, Owner: r.LeaseOwner, Generation: r.LeaseGeneration}
}

type Source struct {
	ID          int64
	RunID       int64
	Handle      string
	Kind        string
	AuthorityID string
	Revision    int64
	PayloadJSON string
	CreatedAtMs int64
}

type Confirmation struct {
	ID                  int64
	UserID              int64
	SessionID           int64
	RunID               int64
	CallID              string
	Tool                string
	CanonicalArgsDigest string
	TargetRevision      int64
	Status              string
	CreatedAtMs         int64
	ResolvedAtMs        int64
}

type QueueItem struct {
	ID          int64
	UserID      int64
	RunID       int64
	MessageID   int64
	CreatedAtMs int64
}

type InputCommand struct {
	ID          int64  `db:"id"`
	UserID      int64  `db:"user_id"`
	RequestID   string `db:"request_id"`
	SessionID   int64  `db:"session_id"`
	MessageID   int64  `db:"message_id"`
	RunID       int64  `db:"run_id"`
	Disposition string `db:"disposition"`
	CreatedAtMs int64  `db:"created_at_ms"`
}

type Alert struct {
	RunID       int64
	Level       string
	Dimension   string
	CreatedAtMs int64
}

type Outbox struct {
	ID          int64
	UserID      int64
	MessageID   int64
	Op          string
	PayloadJSON string
	Published   bool
	CreatedAtMs int64
}

type DeliveryBucket struct {
	ID            int64
	UserID        int64
	WindowStartMs int64
	NotBeforeMs   int64
	Status        string
	HitIDs        []int64
	RunID         int64
	CreatedAtMs   int64
}

type HistorySessionSummary struct {
	SessionID int64
	First     Message
	Last      Message
	LastAtMs  int64
}

func NowMs() int64 { return time.Now().UnixMilli() }

func Preview(text string, maxRunes int) string {
	runes := []rune(text)
	if maxRunes <= 0 {
		maxRunes = 80
	}
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}

func IsTerminalStatus(status string) bool {
	return status == StatusDone || status == StatusError || status == StatusCancelled
}

func PriorityForSource(source string) int {
	switch source {
	case SourceUser:
		return PriorityUser
	case SourceWatch:
		return PriorityWatch
	case SourceMemoryReview:
		return PriorityMemoryReview
	default:
		return 100
	}
}
