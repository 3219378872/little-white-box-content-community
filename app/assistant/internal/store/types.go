package store

import (
	"errors"
	"time"
)

const (
	// Run sources; lower priority values are claimed first.
	SourceUser         = "user"
	SourceMemoryReview = "memory-review"

	PriorityUser         = 0
	PriorityMemoryReview = 20

	// Run phases describe what a running worker is doing; statuses describe the run lifecycle.
	PhaseQueued        = "queued"
	PhaseModelRequest  = "model_request"
	PhaseToolExecuting = "tool_executing"
	PhaseCompact       = "compact"
	PhaseAttachment    = "attachment"
	PhaseDone          = "done"
	PhaseWaitingInput  = "waiting_input"

	StatusQueued         = "queued"
	StatusRunning        = "running"
	StatusDone           = "done"
	StatusError          = "error"
	StatusCancelled      = "cancelled"
	StatusWaitingInput   = "waiting_input"
	StatusWaitingConfirm = "waiting_confirm"

	// Dispositions tell the client how a posted message was absorbed.
	DispositionStarted    = "started"
	DispositionRedirected = "redirected"
	DispositionSteered    = "steered"
	DispositionQueued     = "queued"

	// Event types emitted to the run event stream.
	EventRunStarted        = "run_started"
	EventToken             = "token"
	EventResponseReset     = "response_reset"
	EventProviderAttempt   = "provider_attempt"
	EventToolCall          = "tool_call"
	EventToolResult        = "tool_result"
	EventConfirmRequired   = "confirm_required"
	EventSourceCard        = "source_card"
	EventMemoryChanged     = "memory_changed"
	EventDone              = "done"
	EventError             = "error"
	EventQuestionsRequired = "questions_required"
	EventQuestionsResolved = "questions_resolved"
	EventAnswerCommitted   = "answer_committed"

	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"

	KindMessage       = "message"
	KindMemoryChanged = "memory_changed"
	KindTool          = "tool"
	KindQuestion      = "question"

	SessionOpen   = "open"
	SessionClosed = "closed"

	ConfirmPending  = "pending"
	ConfirmApproved = "approved"
	ConfirmRejected = "rejected"
	ConfirmExpired  = "expired"

	JournalPending = "pending"
	JournalSuccess = "success"
	JournalError   = "error"

	// MaxInputQueue caps messages waiting behind a busy run.
	MaxInputQueue = 32

	// Outbox operations for the message search index.
	IndexOpUpsert = "upsert"
	IndexOpDelete = "delete"
)

// ErrLeaseLost means another worker took over the run; the caller must stop writing.
var ErrLeaseLost = errors.New("assistant run lease lost")

// LeaseFence identifies the worker lease a write is valid under.
type LeaseFence struct {
	RunID      int64
	Owner      string
	Generation int64
}

// Thread is the per-user conversation summary shown in the inbox.
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

// Session holds the prompt and tool snapshots a run is built from; PromptEpoch bumps invalidate them.
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

// Message is one transcript row; APIContent keeps the provider-format payload for replay.
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

// Run is one assistant turn with its lease, cancellation flag and token/cost accounting.
type Run struct {
	ClientProtocolVersion int
	ID                    int64
	UserID                int64
	SessionID             int64
	RequestID             string
	Source                string
	Status                string
	Phase                 string
	Priority              int
	QueuedPayload         []byte
	LeaseOwner            string
	LeaseGeneration       int64
	LeaseUntilMs          int64
	HeartbeatAtMs         int64
	CancelRequested       bool
	ConsentVersion        int32
	InputVersion          int64
	PromptEpoch           int
	Model                 string
	Rounds                int
	ToolCalls             int
	InputTokens           int64
	OutputTokens          int64
	CacheTokens           int64
	CacheWriteTokens      int64
	ReasoningTokens       int64
	LastPromptTokens      int64
	UsageEstimated        bool
	CostUSD               float64
	StartedAtMs           int64
	EndedAtMs             int64
	LastActivityAtMs      int64
	ErrorCode             string
	CreatedAtMs           int64
}

// Event is one item of a run's ordered event stream.
type Event struct {
	ID          int64
	RunID       int64
	Seq         int64
	Type        string
	PayloadJSON []byte
	CreatedAtMs int64
}

// EventPayload is the JSON body of an Event; fields are set per event type.
type EventPayload struct {
	Question   *QuestionRequest    `json:"questionRequest,omitempty"`
	Answer     *AnswerPresentation `json:"answerPresentation,omitempty"`
	Text       string              `json:"text,omitempty"`
	Degraded   bool                `json:"degraded,omitempty"`
	ErrorCode  string              `json:"error_code,omitempty"`
	SessionID  int64               `json:"session_id,omitempty"`
	ToolCall   *ToolInfo           `json:"tool_call,omitempty"`
	SourceCard *SourceRef          `json:"source_card,omitempty"`
	ChangeID   int64               `json:"change_id,omitempty"`
	Partial    string              `json:"partial,omitempty"`
	Journal    string              `json:"journal,omitempty"`
	StreamID   string              `json:"stream_id,omitempty"`
	RouteID    string              `json:"route_id,omitempty"`
	Attempt    int                 `json:"attempt,omitempty"`
	ErrorClass string              `json:"error_class,omitempty"`
	StatusCode int                 `json:"status_code,omitempty"`
	Retryable  bool                `json:"retryable,omitempty"`
}

// ToolInfo describes a tool call in tool_call and confirm_required events.
type ToolInfo struct {
	CallID      string `json:"call_id,omitempty"`
	Tool        string `json:"tool,omitempty"`
	Summary     string `json:"summary,omitempty"`
	PayloadJSON string `json:"payload_json,omitempty"`
}

// SourceRef is a citable source card carried by source_card events.
type SourceRef struct {
	Evidence    []Evidence `json:"evidence,omitempty"`
	Handle      string     `json:"handle"`
	Kind        string     `json:"kind"`
	AuthorityID string     `json:"authority_id"`
	Title       string     `json:"title"`
	Revision    int64      `json:"revision"`
	PayloadJSON string     `json:"payload_json,omitempty"`
	Available   bool       `json:"available,omitempty"`
}

// ToolCall records a model-requested tool invocation and its result.
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

// Journal deduplicates side-effecting tool calls by (user, request, tool, args digest).
// Takeover is set when a newer lease generation reclaimed an unfinished entry.
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

// Fence returns the lease fence of the run as claimed.
func (r Run) Fence() LeaseFence {
	return LeaseFence{RunID: r.ID, Owner: r.LeaseOwner, Generation: r.LeaseGeneration}
}

// Source is a source handle registered in a run's ledger.
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

// Confirmation is a pending or resolved user approval for a tool call.
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

// QueueItem is a user message waiting for the busy run to pick it up.
type QueueItem struct {
	ID          int64
	UserID      int64
	RunID       int64
	MessageID   int64
	CreatedAtMs int64
}

// InputCommand records how a posted message was handled, so retries with the same request ID return the same outcome.
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

// Alert is a deduplicated budget alert for a run dimension.
type Alert struct {
	RunID       int64
	Level       string
	Dimension   string
	CreatedAtMs int64
}

// Outbox is a pending search index change for one message.
type Outbox struct {
	ID          int64
	UserID      int64
	MessageID   int64
	Op          string
	PayloadJSON string
	Published   bool
	CreatedAtMs int64
}

// HistorySessionSummary is the first and last message of a past session, used by history search.
type HistorySessionSummary struct {
	SessionID int64
	First     Message
	Last      Message
	LastAtMs  int64
}

// NowMs is the store's clock in Unix milliseconds.
func NowMs() int64 { return time.Now().UnixMilli() }

// Preview truncates text by runes for thread previews; maxRunes <= 0 means 80.
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

// IsTerminalStatus reports whether a run can no longer change.
func IsTerminalStatus(status string) bool {
	return status == StatusDone || status == StatusError || status == StatusCancelled
}

// PriorityForSource maps a run source to its claim priority; unknown sources go last.
func PriorityForSource(source string) int {
	switch source {
	case SourceUser:
		return PriorityUser
	case SourceMemoryReview:
		return PriorityMemoryReview
	default:
		return 100
	}
}
